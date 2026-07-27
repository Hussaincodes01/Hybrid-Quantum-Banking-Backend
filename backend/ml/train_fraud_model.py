"""
Train FINIX fraud detection model and export to ONNX.

Features mirror RiskSignal struct in risk_engine.go.
Labels: fraud_label (0=low, 1=medium, 2=high)
Synthetic data generation mirrors GenerateSyntheticDataset in aiml.go
and heuristic scoring from risk_engine.go.
"""

import os
import numpy as np
import pandas as pd
from sklearn.model_selection import train_test_split
from sklearn.preprocessing import StandardScaler
from sklearn.metrics import classification_report, confusion_matrix
from xgboost import XGBClassifier
from skl2onnx import convert_sklearn
from skl2onnx.common.data_types import FloatTensorType
import joblib

SEED = 42
N_SAMPLES = 50_000
MODEL_DIR = os.path.join(
    os.path.dirname(__file__), "..", "models", "security", "fraud_label", "v1"
)


def generate_synthetic_fraud_data(n: int, seed: int = SEED) -> pd.DataFrame:
    """
    Generate synthetic fraud detection data mirroring:
    - GenerateSyntheticDataset in aiml.go (feature ranges)
    - risk_engine.go heuristic scoring logic (label assignment)
    """
    rng = np.random.default_rng(seed)

    # --- Feature generation (mirrors aiml.go ranges) ---
    amount_vs_average = rng.uniform(0.1, 10.0, n)
    is_new_recipient = rng.integers(0, 2, n)
    hour_of_day = rng.integers(0, 24, n)
    failed_pin_attempts = rng.integers(0, 6, n)
    velocity_count_1hr = rng.integers(0, 20, n)
    balance_impact = rng.uniform(0.0, 1.0, n)
    recipient_gnn_score = rng.uniform(0.0, 1.0, n)
    behaviour_drift = rng.uniform(0.0, 1.0, n)
    session_trust_score = rng.uniform(0.0, 1.0, n)
    # --- Spec §2.1.1 features (schema parity with FINIX Dataset Features doc) ---
    # geo_velocity_flag: derived from latitude/longitude + last_transaction_* + the
    #   previous timestamp (Haversine/hours > 900 km/h). ~5% impossible-travel.
    geo_velocity_flag = (rng.random(n) < 0.05).astype(np.float64)
    # day_of_week_zscore: (spend(weekday) - benchmark_mean) / benchmark_std.
    day_of_week_zscore = rng.normal(0.0, 1.0, n)
    # market_context_term: realised vol > 2x 30-day avg (market_volatility_index). ~10%.
    market_context_term = (rng.random(n) < 0.10).astype(np.float64)
    # structuring_flag: 3+ uniform sub-benchmark transfers in 24h (new or same recipient). ~3%.
    structuring_flag = (rng.random(n) < 0.03).astype(np.float64)

    # --- Label assignment using risk_engine.go heuristic logic ---
    scores = np.zeros(n, dtype=np.float64)

    # Amount vs average: up to 30 pts
    amount_mask = amount_vs_average > 1.0
    scores[amount_mask] += np.minimum(
        (amount_vs_average[amount_mask] - 1.0) * 12.0, 30.0
    )

    # New recipient: 15 pts
    scores[is_new_recipient == 1] += 15.0

    # Unusual hour (<=4 or >=23): 10 pts
    unusual_hour = (hour_of_day <= 4) | (hour_of_day >= 23)
    scores[unusual_hour] += 10.0

    # Failed PIN attempts: up to 15 pts
    pin_scores = np.minimum(failed_pin_attempts * 5.0, 15.0)
    scores += pin_scores

    # Velocity >= 3: up to 10 pts
    vel_mask = velocity_count_1hr >= 3
    vel_scores = np.zeros(n, dtype=np.float64)
    vel_scores[vel_mask] = np.minimum(velocity_count_1hr[vel_mask] * 2.0, 10.0)
    scores += vel_scores

    # Balance impact: >0.8 -> 10 pts, >0.5 -> 5 pts
    high_drain = balance_impact > 0.8
    med_drain = (balance_impact > 0.5) & (balance_impact <= 0.8)
    scores[high_drain] += 10.0
    scores[med_drain] += 5.0

    # GNN score: up to 20 pts
    scores += np.minimum(recipient_gnn_score * 20.0, 20.0)

    # Behaviour drift: up to 20 pts
    scores += np.minimum(behaviour_drift * 20.0, 20.0)

    # Session trust: <0.4 -> 15 pts, <0.7 -> 8 pts
    low_trust = session_trust_score < 0.4
    mid_trust = (session_trust_score >= 0.4) & (session_trust_score < 0.7)
    scores[low_trust] += 15.0
    scores[mid_trust] += 8.0

    # Spec §2.1.3 rule-based terms (mirror risk_engine.go assess()).
    scores += 20.0 * geo_velocity_flag                                  # impossible travel
    scores += np.minimum(10.0 * np.maximum(0.0, day_of_week_zscore), 10.0)  # day-of-week anomaly, capped at 10
    scores += 8.0 * market_context_term                                 # elevated market volatility
    scores += 25.0 * structuring_flag                                   # structuring/smurfing

    # Cap at 100
    scores = np.minimum(scores, 100.0)

    # Map score -> 3-class label (mirrors risk_engine.go thresholds)
    labels = np.zeros(n, dtype=np.int32)
    labels[scores >= 40] = 1  # medium
    labels[scores >= 70] = 2  # high

    df = pd.DataFrame(
        {
            "amount_vs_average": amount_vs_average,
            "is_new_recipient": is_new_recipient.astype(np.float64),
            "hour_of_day": hour_of_day.astype(np.float64),
            "failed_pin_attempts": failed_pin_attempts.astype(np.float64),
            "velocity_count_1hr": velocity_count_1hr.astype(np.float64),
            "balance_impact": balance_impact,
            "recipient_gnn_score": recipient_gnn_score,
            "behaviour_drift": behaviour_drift,
            "session_trust_score": session_trust_score,
            # Column order below MUST match signalToFeatures() indices 9-12 in
            # risk_engine.go / heuristic.go, or ONNX inference misaligns features.
            "geo_velocity_flag": geo_velocity_flag,
            "day_of_week_zscore": day_of_week_zscore,
            "market_context_term": market_context_term,
            "structuring_flag": structuring_flag,
            "fraud_label": labels,
        }
    )
    return df


def main():
    print("=" * 60)
    print("FINIX Fraud Detection Model Training")
    print("=" * 60)

    # 1. Generate data
    print(f"\nGenerating {N_SAMPLES:,} synthetic samples...")
    df = generate_synthetic_fraud_data(N_SAMPLES)
    print(f"Label distribution:\n{df['fraud_label'].value_counts().sort_index().to_string()}")

    feature_cols = [c for c in df.columns if c != "fraud_label"]
    X = df[feature_cols].values
    y = df["fraud_label"].values

    # 2. Train/test split
    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=SEED, stratify=y
    )
    print(f"\nTrain: {X_train.shape[0]:,}  Test: {X_test.shape[0]:,}")

    # 3. Scale features
    scaler = StandardScaler()
    X_train_scaled = scaler.fit_transform(X_train)
    X_test_scaled = scaler.transform(X_test)

    # 4. Train XGBoost
    print("\nTraining XGBoost classifier...")
    model = XGBClassifier(
        n_estimators=300,
        max_depth=6,
        learning_rate=0.1,
        subsample=0.8,
        colsample_bytree=0.8,
        objective="multi:softprob",
        num_class=3,
        eval_metric="mlogloss",
        random_state=SEED,
        use_label_encoder=False,
    )
    model.fit(
        X_train_scaled,
        y_train,
        eval_set=[(X_test_scaled, y_test)],
        verbose=50,
    )

    # 5. Evaluate
    y_pred = model.predict(X_test_scaled)
    print("\n--- Classification Report ---")
    print(
        classification_report(
            y_test,
            y_pred,
            target_names=["low (0)", "medium (1)", "high (2)"],
        )
    )
    print("--- Confusion Matrix ---")
    print(confusion_matrix(y_test, y_pred))

    # 6. Export to ONNX
    os.makedirs(MODEL_DIR, exist_ok=True)

    initial_type = [("float_input", FloatTensorType([None, len(feature_cols)]))]
    onnx_model = convert_sklearn(model, initial_types=initial_type)
    onnx_path = os.path.join(MODEL_DIR, "model.onnx")
    with open(onnx_path, "wb") as f:
        f.write(onnx_model.SerializeToString())
    print(f"\nONNX model saved: {onnx_path}")

    # 7. Save preprocessor
    preprocess_path = os.path.join(MODEL_DIR, "preprocess.joblib")
    joblib.dump(
        {"scaler": scaler, "feature_columns": feature_cols}, preprocess_path
    )
    print(f"Preprocessor saved: {preprocess_path}")

    # 8. Verify with onnxruntime
    import onnxruntime as ort

    sess = ort.InferenceSession(onnx_path)
    test_input = X_test_scaled[:5].astype(np.float32)
    ort_output = sess.run(None, {"float_input": test_input})
    print(f"\nONNX inference test (first 5 samples):")
    print(f"  Predicted labels: {np.argmax(ort_output[0], axis=1).tolist()}")
    print(f"  Actual labels:    {y_test[:5].tolist()}")

    print("\nDone.")


if __name__ == "__main__":
    main()
