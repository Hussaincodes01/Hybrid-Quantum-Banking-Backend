package platform_test

// Service-level acceptance for the bank provider seam (B5): with the default
// mock injected, beneficiary verification and the payment balance pre-check flow
// through the adapter. External test package so it can import infra/bank without
// an import cycle.

import (
	"testing"

	"FINIX/backend/internal/domain/platform"
	"FINIX/backend/internal/infra/bank"
)

func newServiceWithMockBank(t *testing.T) (*platform.Service, string) {
	t.Helper()
	svc := platform.NewService()
	svc.UseBankAdapter(bank.NewMockBank())
	reg, err := svc.Register(platform.RegisterRequest{
		Name: "Payer", Mobile: "9998887776", DeviceIDFingerprint: "device-seam-1",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return svc, reg.UserID
}

func TestCreateBeneficiaryVerifiesThroughAdapter(t *testing.T) {
	svc, userID := newServiceWithMockBank(t)

	// A seeded UPI resolves -> verified (not the old blanket "pending").
	verified, err := svc.CreateBeneficiary(userID, platform.CreateBeneficiaryRequest{
		BeneficiaryName: "Jiyad", UPIIDOrBankDetails: "jiyad@sbi", RelationshipDescription: "friend",
	})
	if err != nil {
		t.Fatalf("create verified beneficiary: %v", err)
	}
	if verified.VerificationStatus != "verified" {
		t.Errorf("jiyad@sbi verificationStatus = %q, want verified", verified.VerificationStatus)
	}

	// An unknown UPI -> not_found.
	unknown, err := svc.CreateBeneficiary(userID, platform.CreateBeneficiaryRequest{
		BeneficiaryName: "Ghost", UPIIDOrBankDetails: "ghost@fakebank", RelationshipDescription: "unknown",
	})
	if err != nil {
		t.Fatalf("create unknown beneficiary: %v", err)
	}
	if unknown.VerificationStatus != "not_found" {
		t.Errorf("ghost@fakebank verificationStatus = %q, want not_found", unknown.VerificationStatus)
	}
}

func TestPaymentBalancePrecheckThroughAdapter(t *testing.T) {
	svc, userID := newServiceWithMockBank(t)

	// A normal payment passes the balance pre-check and initiates.
	pay, err := svc.InitiatePayment(userID, platform.InitiatePaymentRequest{
		BeneficiaryName: "Venkat", BeneficiaryAccount: "venkat@icici", AmountPaise: 150000, Method: "upi",
	})
	if err != nil {
		t.Fatalf("normal payment should initiate: %v", err)
	}
	if pay.ID == "" {
		t.Error("expected a payment id")
	}

	// An unresolvable payee is flagged (risk raised), not hard-blocked.
	flagged, err := svc.InitiatePayment(userID, platform.InitiatePaymentRequest{
		BeneficiaryName: "Nobody", BeneficiaryAccount: "nobody@ghostbank", AmountPaise: 100000, Method: "upi",
	})
	if err != nil {
		t.Fatalf("unverified payee should be flagged, not error: %v", err)
	}
	if flagged.RiskLevel == "low" {
		t.Errorf("unverified payee risk = %q, expected it elevated (flagged)", flagged.RiskLevel)
	}
}
