package transaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"FINIX/backend/internal/ml/preprocess"
)

// the 32 feature names in the exact model order (notebook FEATURE_COLUMNS).
var txnFeatureOrder = []string{
	"transaction_amount", "amount_vs_average", "velocity_term", "balance_impact",
	"transactions_last_1h", "transactions_last_24h", "account_age_days", "current_balance",
	"monthly_income", "failed_pin_attempts", "session_trust_score", "registered_device_match",
	"rooted_device", "emulator_detected", "otp_verified", "biometric_verified",
	"recipient_seen_before", "recipient_verified", "recipient_account_age_days", "recipient_gnn_score",
	"market_volatility_index", "geo_velocity_flag", "structuring_flag",
	"transaction_type", "payment_channel", "merchant_category", "currency", "account_type",
	"occupation", "kyc_status", "device_type", "authentication_method",
}

// writeIdentityArtifact writes a preprocess artifact with an identity scaler
// (mean 0, scale 1 for all 32 columns) so BuildVector output equals the raw
// mapped value — letting the test assert the RiskSignal->feature wiring directly.
func writeIdentityArtifact(t *testing.T) *preprocess.Artifact {
	t.Helper()
	mean := make([]float64, len(txnFeatureOrder))
	scale := make([]float64, len(txnFeatureOrder))
	for i := range scale {
		scale[i] = 1
	}
	doc := map[string]any{
		"model":          "transaction_risk_model",
		"feature_order":  txnFeatureOrder,
		"scaled_columns": txnFeatureOrder,
		"scaler":         map[string]any{"type": "standard", "mean": mean, "scale": scale},
		"categoricals": map[string][]string{
			"transaction_type":      {"p2p", "transfer"},
			"payment_channel":       {"upi", "neft"},
			"merchant_category":     {"grocery", "utility"},
			"currency":              {"INR", "USD"},
			"account_type":          {"current", "savings"},
			"occupation":            {"engineer", "student"},
			"kyc_status":            {"pending", "verified"},
			"device_type":           {"android", "ios"},
			"authentication_method": {"biometric", "pin"},
		},
	}
	b, _ := json.MarshalIndent(doc, "", "  ")
	path := filepath.Join(t.TempDir(), "transaction_preprocess.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	art, err := preprocess.Load(path)
	if err != nil {
		t.Fatalf("load artifact: %v", err)
	}
	if art.FeatureCount() != txnFeatureCount {
		t.Fatalf("feature count = %d, want %d", art.FeatureCount(), txnFeatureCount)
	}
	return art
}

func idx(name string) int {
	for i, n := range txnFeatureOrder {
		if n == name {
			return i
		}
	}
	return -1
}

func TestFeatureMapsWiring(t *testing.T) {
	art := writeIdentityArtifact(t)
	sig := RiskSignal{
		AmountVsAverage:   3.5,
		IsNewRecipient:    true, // recipient_seen_before -> 0
		VelocityCount1Hr:  4,    // transactions_last_1h -> 4, velocity_term -> min(8,10)=8
		FailedPINAttempts: 2,
		BalanceImpact:     0.6,
		RecipientGNNScore: 0.9,
		SessionTrustScore: 0.3,
		TransactionAmount: 12000,
		CurrentBalance:    50000,
		MonthlyIncome:     80000,
		RootedDevice:      true,
		OTPVerified:       true,
		AccountType:       "savings",     // code 1
		KYCStatus:         "verified",    // code 1
		TransactionType:   "transfer",    // code 1
		MerchantCategory:  "grocery",     // code 0
	}
	num, cat := sig.featureMaps()
	vec, defaulted := art.BuildVector(num, cat)

	checks := map[string]float32{
		"amount_vs_average":       3.5,
		"velocity_term":           8,
		"transactions_last_1h":    4,
		"failed_pin_attempts":     2,
		"balance_impact":          0.6,
		"recipient_gnn_score":     0.9,
		"session_trust_score":     0.3,
		"recipient_seen_before":   0, // is-new -> not seen
		"rooted_device":           1,
		"otp_verified":            1,
		"registered_device_match": 0,
		"transaction_amount":      12000,
		"current_balance":         50000,
		"monthly_income":          80000,
		"account_type":            1,
		"kyc_status":              1,
		"transaction_type":        1,
		"merchant_category":       0,
	}
	for name, want := range checks {
		if got := vec[idx(name)]; got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}

	// Unpopulated continuous/categorical columns must be mean-defaulted (0 here).
	for _, name := range []string{"transactions_last_24h", "account_age_days",
		"market_volatility_index", "currency", "occupation", "device_type",
		"authentication_method", "payment_channel"} {
		if got := vec[idx(name)]; got != 0 {
			t.Errorf("unpopulated %s = %v, want 0 (mean-default)", name, got)
		}
	}
	if len(defaulted) == 0 {
		t.Error("expected some defaulted feature names for the unpopulated columns")
	}
}
