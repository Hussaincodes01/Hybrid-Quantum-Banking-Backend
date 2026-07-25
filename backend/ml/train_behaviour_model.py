"""
Train FINIX user behaviour segmentation model and export to ONNX.

Features mirror PredictBehaviour feature vector in aiml.go.
Labels: segment (balanced_builder, security_sensitive, goal_accelerator, liquidity_focused)
Synthetic data mirrors GenerateSyntheticDataset and segment logic in aiml.go.
"""

import os
import numpy as np
import pandas as pd
from sklearn.model_selection import train_test_split
from sklearn.preprocessing import StandardScaler, LabelEncoder
from sklearn.metrics import classification_report, confusion_matrix
from xgboost import XGBClassifier
from skl2onnx import convert_sklearn
from skl2onnx.common.data_types import FloatTensorType
import joblib

SEED = 42
N_SAMPLES = 50_000
MODEL_DIR = os.path.join(
    os.path.dirname(__file__), "..", "models", "behaviour", "user_segment", "v1"
)

SEGMENT_NAMES = [
    "balanced_builder",
    "security_sensitive",
    "goal_accelerator",
    "liquidity_focused",
]


def generate_synthetic_behaviour_data(n: int, seed: int = SEED) -> pd.DataFrame:
    """
    Generate synthetic behaviour data mirroring:
    - GenerateSyntheticDataset in aiml.go (feature ranges)
    - PredictBehaviour segment assignment logic in aiml.go
    """
    rng = np.random.default_rng(seed)

    # --- Feature generation (mirrors aiml.go SyntheticBehaviourRow) ---
    txn_count = rng.integers(5, 280, n).astype(np.float64)
    high_risk_txn_ratio = rng.uniform(0.0, 0.6, n)
    blocked_txn_ratio = rng.uniform(0.0, 0.4, n)
    late_night_txn_ratio = rng.uniform(0.0, 0.45, n)
    failed_auth_ratio = rng.uniform(0.0, 0.6, n)
    goal_progress_ratio = rng.uniform(0.0, 1.0, n)
    total_balance_lakhs = rng.uniform(0.1, 8.0, n)
    savings_rate = rng.uniform(0.0, 0.8, n)
    monthly_income = rng.uniform(30000, 250000, n) / 10.0  # lakhs

    # --- Segment assignment (mirrors PredictBehaviour in aiml.go) ---
    # Compute derived scores from Go logic
    fraud_prob = np.clip(
        0.08
        + (high_risk_txn_ratio * 0.35)
        + (blocked_txn_ratio * 0.28)
        + (late_night_txn_ratio * 0.16)
        + (failed_auth_ratio * 0.13),
        0.01,
        0.99,
    )
    security_risk = np.clip(fraud_prob * 100.0, 1.0, 99.0)

    stability = np.clip(
        (1.0 - (high_risk_txn_ratio * 0.5 + blocked_txn_ratio * 0.35 + late_night_txn_ratio * 0.15))
        * 100.0,
        5.0,
        98.0,
    )
    spending_discipline = np.clip(
        (1.0 - blocked_txn_ratio * 0.55 - high_risk_txn_ratio * 0.25) * 100.0,
        5.0,
        99.0,
    )
    goal_completion_prob = np.clip(
        goal_progress_ratio * 0.6 + (stability / 100.0) * 0.4, 0.05, 0.98
    )

    # Segment logic from aiml.go lines 213-221
    segments = np.full(n, "balanced_builder", dtype=object)
    segments[security_risk >= 70] = "security_sensitive"
    goal_acc = (goal_completion_prob >= 0.72) & (spending_discipline >= 70)
    segments[goal_acc] = "goal_accelerator"
    liq = total_balance_lakhs <= 4.0
    # Don't override security_sensitive
    liq = liq & (security_risk < 70) & (~goal_acc)
    segments[liq] = "liquidity_focused"

    df = pd.DataFrame(
        {
            "txn_count": txn_count,
            "high_risk_txn_ratio": high_risk_txn_ratio,
            "blocked_txn_ratio": blocked_txn_ratio,
            "late_night_txn_ratio": late_night_txn_ratio,
            "failed_auth_ratio": failed_auth_ratio,
            "goal_progress_ratio": goal_progress_ratio,
            "total_balance_lakhs": total_balance_lakhs,
            "savings_rate": savings_rate,
            "monthly_income": monthly_income,
            "segment": segments,
        }
    )
    return df


def main():
    print("=" * 60)
    print("FINIX User Behaviour Segmentation Training")
    print("=" * 60)

    # 1. Generate data
    print(f"\nGenerating {N_SAMPLES:,} synthetic samples...")
    df = generate_synthetic_behaviour_data(N_SAMPLES)
    print(f"Segment distribution:\n{df['segment'].value_counts().to_string()}")

    feature_cols = [c for c in df.columns if c != "segment"]
    X = df[feature_cols].values

    le = LabelEncoder()
    y = le.fit_transform(df["segment"].values)
    print(f"\nClasses: {le.classes_.tolist()}")

    # 2. Train/test split
    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=SEED, stratify=y
    )
    print(f"Train: {X_train.shape[0]:,}  Test: {X_test.shape[0]:,}")

    # 3. Scale
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
        num_class=len(SEGMENT_NAMES),
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
            target_names=le.classes_,
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

    # 7. Save preprocessor + label encoder
    preprocess_path = os.path.join(MODEL_DIR, "preprocess.joblib")
    joblib.dump(
        {
            "scaler": scaler,
            "feature_columns": feature_cols,
            "label_encoder": le,
        },
        preprocess_path,
    )
    print(f"Preprocessor saved: {preprocess_path}")

    # 8. Verify with onnxruntime
    import onnxruntime as ort

    sess = ort.InferenceSession(onnx_path)
    test_input = X_test_scaled[:5].astype(np.float32)
    ort_output = sess.run(None, {"float_input": test_input})
    predicted_indices = np.argmax(ort_output[0], axis=1)
    predicted_labels = le.inverse_transform(predicted_indices)
    actual_labels = le.inverse_transform(y_test[:5])
    print(f"\nONNX inference test (first 5 samples):")
    print(f"  Predicted: {predicted_labels.tolist()}")
    print(f"  Actual:    {actual_labels.tolist()}")

    print("\nDone.")


if __name__ == "__main__":
    main()
