package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

func TestPhase1AuthAndKYCProfiles(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{
		Name:                "Asha Kumar",
		Mobile:              "9999999999",
		Email:               "asha@example.com",
		DeviceIDFingerprint: "dev-fp-123",
		DeviceType:          "android",
		AppVersion:          "1.2.3",
		IPAddress:           "10.0.0.1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	authProfile, err := svc.AuthProfile(reg.UserID)
	if err != nil {
		t.Fatalf("auth profile fetch failed: %v", err)
	}
	if authProfile.Email != "asha@example.com" {
		t.Fatalf("expected email to be persisted, got %q", authProfile.Email)
	}
	if !authProfile.TrustedDeviceFlag {
		t.Fatal("expected trusted device flag to default true")
	}

	_, err = svc.VerifyEKYC(EKYCRequest{UserID: reg.UserID, PanLast4: "1234", AadhaarLast4: "1234"})
	if err != nil {
		t.Fatalf("ekyc verify failed: %v", err)
	}

	kycProfile, err := svc.KYCProfile(reg.UserID)
	if err != nil {
		t.Fatalf("kyc profile fetch failed: %v", err)
	}
	if kycProfile.KYCStatus != "verified" {
		t.Fatalf("expected verified kyc status, got %q", kycProfile.KYCStatus)
	}
	if kycProfile.PANMasked == "" || kycProfile.AadhaarMaskedOrHash == "" {
		t.Fatal("expected PAN/Aadhaar masked values after ekyc")
	}

	trusted := false
	stepUp := true
	authProfile, err = svc.UpdateAuthProfile(reg.UserID, UpdateAuthProfileRequest{
		IPAddress:              "10.0.0.2",
		TrustedDeviceFlag:      &trusted,
		StepUpAuthRequiredFlag: &stepUp,
	})
	if err != nil {
		t.Fatalf("update auth profile failed: %v", err)
	}
	if authProfile.TrustedDeviceFlag {
		t.Fatal("expected trusted device flag update to false")
	}
	if !authProfile.StepUpAuthRequiredFlag {
		t.Fatal("expected step-up auth flag true")
	}

	updatedKYC, err := svc.UpsertKYCProfile(reg.UserID, UpsertKYCProfileRequest{
		Occupation:            "Engineer",
		Employer:              "PSB Labs",
		AnnualIncome:          "1200000",
		TaxResidency:          "India",
		NomineeDetails:        "Mother",
		KYCVerificationMethod: "video",
		KYCDocumentTypes:      []string{"PAN", "AADHAAR"},
		KYCDocumentFiles:      []string{"pan.pdf", "aadhaar.pdf"},
	})
	if err != nil {
		t.Fatalf("upsert kyc profile failed: %v", err)
	}
	if updatedKYC.Employer != "PSB Labs" {
		t.Fatalf("expected employer update, got %q", updatedKYC.Employer)
	}
	if updatedKYC.KYCSubmissionDate == nil {
		t.Fatal("expected kyc submission timestamp")
	}

	// Generate real ECDSA P-256 key pair for biometric
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key pair failed: %v", err)
	}
	pubKeyBytes := elliptic.Marshal(privKey.Curve, privKey.PublicKey.X, privKey.PublicKey.Y)
	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	_, err = svc.RegisterBiometric(BiometricSetupRequest{UserID: reg.UserID, PublicKeyB64: pubKeyB64})
	if err != nil {
		t.Fatalf("biometric setup failed: %v", err)
	}
	challenge, err := svc.CreateChallenge(LoginChallengeRequest{UserID: reg.UserID})
	if err != nil {
		t.Fatalf("create challenge failed: %v", err)
	}

	svc.mu.RLock()
	keyID := svc.authProfiles[reg.UserID].DeviceChallengePublicKeyID
	svc.mu.RUnlock()

	challengeBytes, _ := base64.StdEncoding.DecodeString(challenge.Challenge)
	digest := sha256.Sum256(challengeBytes)
	sig, _ := ecdsa.SignASN1(rand.Reader, privKey, digest[:])
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	login, err := svc.VerifyLogin(LoginVerifyRequest{
		UserID:              reg.UserID,
		ChallengeID:         challenge.ChallengeID,
		Signature:           sigB64,
		KeyID:               keyID,
		DeviceIDFingerprint: "dev-fp-123",
	})
	if err != nil {
		t.Fatalf("verify login failed: %v", err)
	}
	if _, ok := svc.AuthenticateToken(login.AccessToken, "dev-fp-123"); !ok {
		t.Fatal("expected issued token to authenticate")
	}

	svc.mu.Lock()
	svc.sessionExpiry[login.AccessToken] = time.Now().Add(-time.Minute)
	svc.mu.Unlock()
	if _, ok := svc.AuthenticateToken(login.AccessToken, "dev-fp-123"); ok {
		t.Fatal("expected expired token to fail authentication")
	}
}

func TestPhase2AccountsBeneficiariesAndAggregator(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{Name: "Ravi", Mobile: "8888888888", DeviceIDFingerprint: "dev-fp-ravi"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	accounts, err := svc.Accounts(reg.UserID)
	if err != nil {
		t.Fatalf("list accounts failed: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected seeded account, got %d", len(accounts))
	}

	linked, err := svc.LinkAccount(reg.UserID, LinkAccountRequest{
		AccountHolderName:         "Ravi",
		BankName:                  "Demo Bank",
		Branch:                    "MG Road",
		IFSCCode:                  "DEMO0001234",
		MaskedAccount:             "XXXXXX2222",
		UPIID:                     "ravi@upi",
		AccountType:               "current",
		VerificationStatus:        "verified",
		Nickname:                  "Business",
		PrimaryAccountFlag:        true,
		AccountToken:              "accttok_custom",
		TokenProvider:             "fip",
		AccountVerificationMethod: "penny_drop",
		BalancePaise:              4200000,
	})
	if err != nil {
		t.Fatalf("link account failed: %v", err)
	}
	if !linked.PrimaryAccountFlag {
		t.Fatal("expected linked account to be marked primary")
	}

	primary := true
	updated, err := svc.UpdateAccount(reg.UserID, linked.ID, UpdateAccountRequest{Nickname: "Primary Business", PrimaryAccountFlag: &primary})
	if err != nil {
		t.Fatalf("update account failed: %v", err)
	}
	if updated.Nickname != "Primary Business" {
		t.Fatalf("expected updated nickname, got %q", updated.Nickname)
	}

	beneficiary, err := svc.CreateBeneficiary(reg.UserID, CreateBeneficiaryRequest{
		BeneficiaryName:         "Anita",
		RelationshipDescription: "Sister",
		UPIIDOrBankDetails:      "anita@upi",
		TrustScore:              92,
		Notes:                   "Family beneficiary",
	})
	if err != nil {
		t.Fatalf("create beneficiary failed: %v", err)
	}

	approved, err := svc.ApproveBeneficiary(reg.UserID, beneficiary.ID, ApproveBeneficiaryRequest{ApproverID: "ops-1", Approved: true})
	if err != nil {
		t.Fatalf("approve beneficiary failed: %v", err)
	}
	if approved.VerificationStatus != "approved" {
		t.Fatalf("expected approved beneficiary, got %q", approved.VerificationStatus)
	}

	beneficiaries, err := svc.Beneficiaries(reg.UserID)
	if err != nil {
		t.Fatalf("list beneficiaries failed: %v", err)
	}
	if len(beneficiaries) != 1 {
		t.Fatalf("expected 1 beneficiary, got %d", len(beneficiaries))
	}

	status, err := svc.AggregatorStatus(reg.UserID)
	if err != nil {
		t.Fatalf("aggregator status failed: %v", err)
	}
	if status.NumberOfAccountsAggregated != 2 {
		t.Fatalf("expected 2 aggregated accounts, got %d", status.NumberOfAccountsAggregated)
	}

	synced, err := svc.SyncAggregator(reg.UserID, SyncAggregatorRequest{
		LinkedInstitutionID: "fip-001",
		ConnectorStatus:     "healthy",
	})
	if err != nil {
		t.Fatalf("aggregator sync failed: %v", err)
	}
	if synced.LinkedInstitutionID != "fip-001" {
		t.Fatalf("expected linked institution id update, got %q", synced.LinkedInstitutionID)
	}
	if synced.LastSyncTimestamp == nil {
		t.Fatal("expected sync timestamp")
	}
}
