package fraud

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"FINIX/backend/internal/ml/preprocess"
)

var muleFeatureOrder = []string{
	"account_type", "account_age_days", "current_balance", "avg_monthly_balance",
	"monthly_income", "occupation", "kyc_status", "account_status", "home_city",
	"home_state", "home_country", "in_degree", "out_degree", "total_degree",
	"unique_senders", "unique_receivers", "total_incoming_amount",
	"avg_incoming_amount", "total_outgoing_amount", "avg_outgoing_amount",
}

// writeIdentityMuleArtifact builds a 20-feature identity-scaler artifact so
// encoder output equals the raw mapped value (categoricals -> their code).
func writeIdentityMuleArtifact(t *testing.T) *preprocess.Artifact {
	t.Helper()
	mean := make([]float64, len(muleFeatureOrder))
	scale := make([]float64, len(muleFeatureOrder))
	for i := range scale {
		scale[i] = 1
	}
	doc := map[string]any{
		"model":          "MuleAccountDetection",
		"feature_order":  muleFeatureOrder,
		"scaled_columns": muleFeatureOrder,
		"scaler":         map[string]any{"type": "standard", "mean": mean, "scale": scale},
		"categoricals": map[string][]string{
			"account_type":   {"Business", "Current", "Salary", "Savings"},
			"occupation":     {"Student", "Salaried"},
			"kyc_status":     {"Pending", "Rejected", "Verified"},
			"account_status": {"Active", "Dormant", "Frozen"},
			"home_city":      {"Delhi", "Mumbai"},
			"home_state":     {"Delhi", "Maharashtra"},
			"home_country":   {"India"},
		},
	}
	b, _ := json.MarshalIndent(doc, "", "  ")
	path := filepath.Join(t.TempDir(), "mule_preprocess.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	a, err := preprocess.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return a
}

func muleIdx(name string) int {
	for i, n := range muleFeatureOrder {
		if n == name {
			return i
		}
	}
	return -1
}

func TestEncodeMuleNodeUsesArtifact(t *testing.T) {
	prev := muleEncoder
	t.Cleanup(func() { muleEncoder = prev })

	SetMuleEncoder(writeIdentityMuleArtifact(t))
	if !MuleEncoderReady() {
		t.Fatal("encoder should be ready after SetMuleEncoder")
	}

	row, err := encodeMuleNode(MuleAccount{
		ID: "acc-1",
		Raw: map[string]float64{
			"account_age_days": 800, "current_balance": 50000,
			"in_degree": 3, "out_degree": 5, "total_degree": 8,
			"total_incoming_amount": 12000,
		},
		Cat: map[string]string{
			"account_type": "Savings",  // code 3
			"kyc_status":   "Verified", // code 2
			"account_status": "Frozen", // code 2
		},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(row) != muleNodeFeatureWidth {
		t.Fatalf("width %d, want %d", len(row), muleNodeFeatureWidth)
	}
	checks := map[string]float32{
		"account_type":          3,
		"kyc_status":            2,
		"account_status":        2,
		"account_age_days":      800,
		"current_balance":       50000,
		"in_degree":             3,
		"out_degree":            5,
		"total_degree":          8,
		"total_incoming_amount": 12000,
	}
	for name, want := range checks {
		if got := row[muleIdx(name)]; got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	// Unpopulated columns mean-default to 0 under identity scaler.
	for _, name := range []string{"monthly_income", "home_city", "avg_outgoing_amount"} {
		if got := row[muleIdx(name)]; got != 0 {
			t.Errorf("unpopulated %s = %v, want 0", name, got)
		}
	}
}

func TestScoreMuleAccountsFailsSafeWhenEncoderUnset(t *testing.T) {
	prev := muleEncoder
	t.Cleanup(func() { muleEncoder = prev })
	muleEncoder = nil // simulate artifact absent

	// A non-nil "available" predictor so we exercise the encoding gate, not the
	// predictor gate.
	scores, err := ScoreMuleAccounts(stubAvailablePredictor{}, MuleGraphInput{
		Accounts:  []MuleAccount{{ID: "a"}, {ID: "b"}},
		Transfers: []MuleTransfer{{From: "a", To: "b"}},
	})
	if err != nil {
		t.Fatalf("expected fail-safe nil error, got %v", err)
	}
	if scores != nil {
		t.Fatalf("expected nil scores (fail-safe), got %v", scores)
	}
}

type stubAvailablePredictor struct{}

func (stubAvailablePredictor) Predict(nf []float32, nn int, ei []int64, ne int) ([]float32, error) {
	return make([]float32, nn*muleClassCount), nil
}
func (stubAvailablePredictor) IsAvailable() bool { return true }
