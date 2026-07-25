package platform

import (
	"strings"
	"testing"
)

func TestExpandedUPIPinAndInvestmentOrder(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{Name: "Expanded User", Mobile: "9000000001", DeviceIDFingerprint: "dev-exp-1"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, err := svc.VerifyEKYC(EKYCRequest{UserID: reg.UserID, PanLast4: "1234", AadhaarLast4: "1234"}); err != nil {
		t.Fatalf("ekyc failed: %v", err)
	}

	// Generate real ECDSA key pair for biometric
	privKey, pubKeyB64, keyID := generateTestKeyPairAndID(t)
	if _, err := svc.RegisterBiometric(BiometricSetupRequest{UserID: reg.UserID, PublicKeyB64: pubKeyB64, KeyID: keyID}); err != nil {
		t.Fatalf("biometric register failed: %v", err)
	}

	accounts, err := svc.Accounts(reg.UserID)
	if err != nil || len(accounts) == 0 {
		t.Fatalf("expected seeded account, err=%v count=%d", err, len(accounts))
	}
	accountID := accounts[0].ID

	// Generate real OTP
	otp, err := svc.GenerateOTP(reg.UserID)
	if err != nil {
		t.Fatalf("generate OTP failed: %v", err)
	}

	_, err = svc.HandleExpandedEndpoint(reg.UserID, "upi_pin_set", EndpointInput{
		PathParams: map[string]string{"accountID": accountID},
		Body: map[string]any{
			"otp":         otp,
			"new_pin":     "1234",
			"confirm_pin": "1234",
		},
	})
	if err != nil {
		t.Fatalf("upi pin set failed: %v", err)
	}

	// Investment order above 25000*100 requires biometric — should fail without it
	_, err = svc.HandleExpandedEndpoint(reg.UserID, "investment_order", EndpointInput{
		Body: map[string]any{
			"amount":     int64(2600000),
			"order_type": "sell",
			"instrument": "unknown urgent",
		},
	})
	if err == nil {
		t.Fatal("expected biometric error for high amount order without challenge")
	}

	// Create step-up challenge and sign it
	challengeID, challengeB64, err := svc.CreateStepUpChallenge(reg.UserID)
	if err != nil {
		t.Fatalf("create stepup challenge failed: %v", err)
	}
	signatureB64 := signChallengeBase64(t, privKey, challengeB64)

	res, err := svc.HandleExpandedEndpoint(reg.UserID, "investment_order", EndpointInput{
		Body: map[string]any{
			"amount":     int64(2600000),
			"order_type": "sell",
			"instrument": "unknown urgent",
		},
		Headers: map[string]string{
			"X-Biometric-Challenge-Id": challengeID,
			"X-Biometric-Challenge":    signatureB64,
		},
	})
	if err != nil {
		t.Fatalf("investment order with biometric failed: %v", err)
	}
	order, ok := res.(map[string]any)
	if !ok {
		t.Fatal("expected map response for investment order")
	}
	status := order["status"].(string)
	if status == "" {
		t.Fatal("expected non-empty order status")
	}
}

func TestExpandedOTPFailureLock(t *testing.T) {
	svc := NewService()
	reg, err := svc.Register(RegisterRequest{Name: "OTP Lock User", Mobile: "9000000002", DeviceIDFingerprint: "dev-otp-1"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Generate OTP first
	_, err = svc.GenerateOTP(reg.UserID)
	if err != nil {
		t.Fatalf("generate OTP failed: %v", err)
	}

	// Try wrong OTP 3 times — should lock
	for i := 0; i < 2; i++ {
		if err := svc.VerifyOTPToken(reg.UserID, "000000"); err == nil {
			t.Fatal("expected invalid otp error")
		}
	}
	err = svc.VerifyOTPToken(reg.UserID, "000000")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "locked") {
		t.Fatalf("expected lockout error, got: %v", err)
	}
}

func TestExpandedSecurityURLScan(t *testing.T) {
	svc := NewService()
	reg, err := svc.Register(RegisterRequest{Name: "Scanner User", Mobile: "9000000003", DeviceIDFingerprint: "dev-scan-1"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	res, err := svc.HandleExpandedEndpoint(reg.UserID, "security_url_scan", EndpointInput{
		Body: map[string]any{"url": "https://bit.ly/free-money-urgent"},
	})
	if err != nil {
		t.Fatalf("url scan failed: %v", err)
	}
	payload := res.(map[string]any)
	if payload["verdict"].(string) == "safe" {
		t.Fatalf("expected suspicious or blocked verdict, got %v", payload["verdict"])
	}
}
