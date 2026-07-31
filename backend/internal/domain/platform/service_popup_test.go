package platform

import "testing"

// TestRiskScoreDrivesPopup verifies the end-to-end contract the client renders a
// popup from: a low-risk transaction completes silently, while a high-risk one
// (driven by the transaction risk score) returns a step-up status the app shows as
// a warning / confirmation popup. This exercises score -> tier -> status/stepUp.
func TestRiskScoreDrivesPopup(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{
		Name:                "Popup Tester",
		Mobile:              "7000000001",
		Email:               "popup@example.com",
		DeviceIDFingerprint: "dev-popup",
		DeviceType:          "android",
		AppVersion:          "1.0.0",
		IPAddress:           "10.0.0.1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Low-risk: amount at/below the ₹500 population floor (so a brand-new user with
	// no personal history isn't flagged for a modest transfer), trusted session,
	// low drift -> no popup.
	low, err := svc.InitiateTransaction(reg.UserID, "popup-low", InitiateTransactionRequest{
		AmountPaise:       30000, // ₹300, below the FLOOR_AVG baseline
		Recipient:         "known friend",
		Channel:           "upi",
		SessionTrustScore: 0.95,
		BehaviourDrift:    0.05,
	})
	if err != nil {
		t.Fatalf("low-risk initiate failed: %v", err)
	}
	if low.Status != "success" || low.StepUpRequired {
		t.Fatalf("low-risk txn should NOT pop up: status=%q stepUp=%v score=%.1f",
			low.Status, low.StepUpRequired, low.RiskScore)
	}

	// High-risk: large amount to a new recipient, low trust, high drift, failed PINs
	// -> the score should escalate the tier and force a step-up popup.
	high, err := svc.InitiateTransaction(reg.UserID, "popup-high", InitiateTransactionRequest{
		AmountPaise:       9000000, // ₹90,000
		Recipient:         "unknown urgent payee",
		Channel:           "bank_transfer",
		SessionTrustScore: 0.15,
		BehaviourDrift:    0.85,
		FailedPINAttempts: 3,
	})
	if err != nil {
		t.Fatalf("high-risk initiate failed: %v", err)
	}

	if !high.StepUpRequired {
		t.Fatalf("high-risk txn MUST pop up (stepUp): status=%q score=%.1f", high.Status, high.RiskScore)
	}
	if high.Status != "warning_ack_required" && high.Status != "blocked" {
		t.Fatalf("high-risk status should be a popup status, got %q", high.Status)
	}
	if high.RiskScore <= low.RiskScore {
		t.Fatalf("popup must be score-driven: high score %.1f should exceed low score %.1f",
			high.RiskScore, low.RiskScore)
	}
	t.Logf("popup verified: low(status=%s,score=%.1f) high(status=%s,score=%.1f,stepUp=%v)",
		low.Status, low.RiskScore, high.Status, high.RiskScore, high.StepUpRequired)
}
