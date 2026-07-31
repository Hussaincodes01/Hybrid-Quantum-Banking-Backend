package preprocess

import (
	"os"
	"testing"
)

// These tests parse the actual artifacts shipped in backend/models to lock the
// real training contract. They skip (not fail) when the files are absent so the
// suite stays green in trimmed checkouts.

func loadRealOrSkip(t *testing.T, rel string) *Artifact {
	t.Helper()
	if _, err := os.Stat(rel); err != nil {
		t.Skipf("artifact %s not present: %v", rel, err)
	}
	a, err := Load(rel)
	if err != nil {
		t.Fatalf("Load(%s): %v", rel, err)
	}
	return a
}

func TestRealTransactionArtifact(t *testing.T) {
	a := loadRealOrSkip(t, "../../../models/transaction_preprocess.json")

	if a.FeatureCount() != 32 {
		t.Fatalf("feature count = %d, want 32", a.FeatureCount())
	}
	// StandardScaler was fit on 27 columns; the 5 boolean device-posture flags
	// were bool dtype at training time and passed through unscaled.
	if len(a.ScaledColumns) != 27 {
		t.Errorf("scaled columns = %d, want 27", len(a.ScaledColumns))
	}
	for _, c := range []string{
		"registered_device_match", "rooted_device", "emulator_detected",
		"otp_verified", "biometric_verified",
	} {
		if _, scaled := a.scaleByCol[c]; scaled {
			t.Errorf("%s must be unscaled (passed through raw)", c)
		}
	}
	// 9 categoricals, all label-encoded then scaled.
	if len(a.Categoricals) != 9 {
		t.Errorf("categoricals = %d, want 9", len(a.Categoricals))
	}
	if _, scaled := a.scaleByCol["kyc_status"]; !scaled {
		t.Error("kyc_status should be scaled")
	}
	// Encoder vocab round-trip (classes are in code order).
	if code, ok := a.Encode("kyc_status", "Verified"); !ok || code != 2 {
		t.Errorf("Encode(kyc_status, Verified) = %d,%v, want 2,true", code, ok)
	}
	if code, ok := a.Encode("account_type", "Savings"); !ok || code != 3 {
		t.Errorf("Encode(account_type, Savings) = %d,%v, want 3,true", code, ok)
	}
	if a.DecisionThresholdFraud != 0.70 {
		t.Errorf("decision threshold = %v, want 0.70", a.DecisionThresholdFraud)
	}
}

func TestRealMuleArtifact(t *testing.T) {
	a := loadRealOrSkip(t, "../../../models/mule_preprocess.json")

	if a.FeatureCount() != 20 {
		t.Fatalf("feature count = %d, want 20", a.FeatureCount())
	}
	if len(a.ScaledColumns) != 20 {
		t.Errorf("scaled columns = %d, want 20 (all)", len(a.ScaledColumns))
	}
	if len(a.Categoricals) != 7 {
		t.Errorf("categoricals = %d, want 7", len(a.Categoricals))
	}
	if code, ok := a.Encode("account_status", "Frozen"); !ok || code != 2 {
		t.Errorf("Encode(account_status, Frozen) = %d,%v, want 2,true", code, ok)
	}
}
