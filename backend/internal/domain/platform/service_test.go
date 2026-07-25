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

func TestRegisterAndTransactionRiskFlow(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{Name: "Asha", Mobile: "9999999999", DeviceIDFingerprint: "device-asha-1"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err = svc.VerifyEKYC(EKYCRequest{UserID: reg.UserID, PanLast4: "1234", AadhaarLast4: "1234"})
	if err != nil {
		t.Fatalf("ekyc failed: %v", err)
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

	// Get keyID from auth profile
	svc.mu.RLock()
	keyID := svc.authProfiles[reg.UserID].DeviceChallengePublicKeyID
	svc.mu.RUnlock()
	if keyID == "" {
		t.Fatal("expected keyID to be set after biometric registration")
	}

	challenge, err := svc.CreateChallenge(LoginChallengeRequest{UserID: reg.UserID})
	if err != nil {
		t.Fatalf("challenge failed: %v", err)
	}

	// Sign the challenge with the private key
	challengeBytes, _ := base64.StdEncoding.DecodeString(challenge.Challenge)
	digest := sha256.Sum256(challengeBytes)
	sig, err := ecdsa.SignASN1(rand.Reader, privKey, digest[:])
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	login, err := svc.VerifyLogin(LoginVerifyRequest{
		UserID:              reg.UserID,
		ChallengeID:         challenge.ChallengeID,
		Signature:           sigB64,
		KeyID:               keyID,
		DeviceIDFingerprint: "device-asha-1",
	})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if login.AccessToken == "" {
		t.Fatal("expected non-empty access token")
	}

	// Verify token authenticates with device fingerprint
	if _, ok := svc.AuthenticateToken(login.AccessToken, "device-asha-1"); !ok {
		t.Fatal("expected token to authenticate with correct device")
	}

	// Verify token fails with wrong device
	if _, ok := svc.AuthenticateToken(login.AccessToken, "wrong-device"); ok {
		t.Fatal("expected token to fail with wrong device fingerprint")
	}

	low, err := svc.InitiateTransaction(reg.UserID, "idem-low", InitiateTransactionRequest{
		AmountPaise:       15000,
		Recipient:         "known friend",
		Channel:           "upi",
		SessionTrustScore: 0.95,
		BehaviourDrift:    0.05,
	})
	if err != nil {
		t.Fatalf("low transaction failed: %v", err)
	}
	if low.Status == "blocked" {
		t.Fatalf("expected low-risk transaction not to be blocked")
	}

	high, err := svc.InitiateTransaction(reg.UserID, "idem-high", InitiateTransactionRequest{
		AmountPaise:       7500000,
		Recipient:         "unknown urgent account",
		Channel:           "bank_transfer",
		SessionTrustScore: 0.2,
		BehaviourDrift:    0.9,
		FailedPINAttempts: 3,
	})
	if err != nil {
		t.Fatalf("high transaction failed: %v", err)
	}
	if high.Status != "blocked" {
		t.Fatalf("expected blocked high-risk transaction, got %s", high.Status)
	}
	if high.CoolingOffUntil == nil || high.CoolingOffUntil.Before(time.Now()) {
		t.Fatal("expected cooling off timestamp for blocked transaction")
	}
}

func TestRegisterRejectsEmptyDeviceFingerprint(t *testing.T) {
	svc := NewService()
	_, err := svc.Register(RegisterRequest{Name: "Test", Mobile: "7777777777"})
	if err == nil {
		t.Fatal("expected error when registering without device fingerprint")
	}
}

func TestOTPGenerationAndVerification(t *testing.T) {
	svc := NewService()
	reg, err := svc.Register(RegisterRequest{Name: "OTP Test", Mobile: "6666666666", DeviceIDFingerprint: "device-otp-1"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	otp, err := svc.GenerateOTP(reg.UserID)
	if err != nil {
		t.Fatalf("generate OTP failed: %v", err)
	}
	if len(otp) != 6 {
		t.Fatalf("expected 6-digit OTP, got %q", otp)
	}

	// Wrong OTP should fail
	if err := svc.VerifyOTPToken(reg.UserID, "000000"); err == nil {
		t.Fatal("expected wrong OTP to fail")
	}

	// Correct OTP should pass
	if err := svc.VerifyOTPToken(reg.UserID, otp); err != nil {
		t.Fatalf("correct OTP failed: %v", err)
	}

	// OTP should be single-use — second attempt should fail
	if err := svc.VerifyOTPToken(reg.UserID, otp); err == nil {
		t.Fatal("expected used OTP to fail on second attempt")
	}
}

func TestOTPBackdoorRemoved(t *testing.T) {
	svc := NewService()
	reg, _ := svc.Register(RegisterRequest{Name: "Backdoor Test", Mobile: "5555555555", DeviceIDFingerprint: "device-bd-1"})

	// "123456" should NOT work anymore
	if err := svc.VerifyOTPToken(reg.UserID, "123456"); err == nil {
		t.Fatal("expected '123456' backdoor to be rejected")
	}

	// "otp_*" prefix should NOT work anymore
	if err := svc.VerifyOTPToken(reg.UserID, "otp_anything"); err == nil {
		t.Fatal("expected 'otp_*' prefix backdoor to be rejected")
	}
}
