package platform

import (
	"testing"
)

func TestInitiateAndListPayments(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{
		Name:                "Ravi",
		Mobile:              "8888888888",
		DeviceIDFingerprint: "device-ravi-1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	pay, err := svc.InitiatePayment(reg.UserID, InitiatePaymentRequest{
		BeneficiaryName:    "Priya",
		BeneficiaryAccount: "UPI:priya@bank",
		AmountPaise:        500000,
		Currency:           "INR",
		Method:             "upi",
		UserConsent:        true,
	})
	if err != nil {
		t.Fatalf("initiate payment failed: %v", err)
	}
	if pay.ID == "" {
		t.Fatal("expected non-empty payment ID")
	}
	if pay.Status != "initiated" {
		t.Fatalf("expected status 'initiated', got '%s'", pay.Status)
	}
	if pay.AmountPaise != 500000 {
		t.Fatalf("expected amount 500000, got %d", pay.AmountPaise)
	}
	if pay.Currency != "INR" {
		t.Fatalf("expected currency INR, got '%s'", pay.Currency)
	}
	if pay.Method != "upi" {
		t.Fatalf("expected method upi, got '%s'", pay.Method)
	}
	if !pay.UserConsent {
		t.Fatal("expected user consent true")
	}

	list, err := svc.ListPayments(reg.UserID)
	if err != nil {
		t.Fatalf("list payments failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 payment, got %d", len(list))
	}

	// Verify with receipt
	receipt, err := svc.VerifyPaymentReceipt(reg.UserID, pay.ID, VerifyReceiptRequest{
		ReceiptHash:  "0xabc123",
		VerifiedBy:   "bank",
		VerifierType: "bank",
	})
	if err != nil {
		t.Fatalf("verify payment receipt failed: %v", err)
	}
	if receipt.Status != "completed" {
		t.Fatalf("expected status 'completed', got '%s'", receipt.Status)
	}
	if receipt.ReceiptVerification == nil || !receipt.ReceiptVerification.Verified {
		t.Fatal("expected receipt verification to be true")
	}
	if receipt.ReceiptVerification.ReceiptHash != "0xabc123" {
		t.Fatalf("expected receipt hash '0xabc123', got '%s'", receipt.ReceiptVerification.ReceiptHash)
	}
}

func TestInitiatePaymentValidation(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{
		Name:                "Test",
		Mobile:              "7777777777",
		DeviceIDFingerprint: "device-test-1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Zero amount
	_, err = svc.InitiatePayment(reg.UserID, InitiatePaymentRequest{
		BeneficiaryName: "Priya",
		AmountPaise:     0,
	})
	if err == nil {
		t.Fatal("expected error for zero amount")
	}

	// Missing beneficiary
	_, err = svc.InitiatePayment(reg.UserID, InitiatePaymentRequest{
		AmountPaise: 10000,
	})
	if err == nil {
		t.Fatal("expected error for missing beneficiary")
	}

	// Unknown user
	_, err = svc.InitiatePayment("unknown", InitiatePaymentRequest{
		BeneficiaryName: "Priya",
		AmountPaise:     10000,
	})
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
}

func TestGetPaymentNotFound(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{
		Name:                "Neha",
		Mobile:              "6666666666",
		DeviceIDFingerprint: "device-neha-1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err = svc.GetPayment(reg.UserID, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent payment")
	}
}

func TestInitiatePaymentWithQRAndIntent(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{
		Name:                "QR Test",
		Mobile:              "5555555555",
		DeviceIDFingerprint: "device-qr-1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	pay, err := svc.InitiatePayment(reg.UserID, InitiatePaymentRequest{
		BeneficiaryName:    "Merchant",
		BeneficiaryAccount: "merchant@paytm",
		AmountPaise:        250000,
		Currency:           "INR",
		Method:             "qr",
		QRCodePayload:      "upi://pay?pa=merchant@paytm&am=2500&tn=Test",
		PaymentIntentID:    "pi_test123",
		UserConsent:        true,
	})
	if err != nil {
		t.Fatalf("initiate payment with QR failed: %v", err)
	}
	if pay.QRCodePayload == "" {
		t.Fatal("expected QR code payload")
	}
	if pay.PaymentIntentID != "pi_test123" {
		t.Fatalf("expected payment intent ID 'pi_test123', got '%s'", pay.PaymentIntentID)
	}
	if pay.Method != "qr" {
		t.Fatalf("expected method 'qr', got '%s'", pay.Method)
	}
}

func TestVerifyReceiptAlreadyVerified(t *testing.T) {
	svc := NewService()

	reg, err := svc.Register(RegisterRequest{
		Name:                "Double Verify",
		Mobile:              "4444444444",
		DeviceIDFingerprint: "device-dv-1",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	pay, err := svc.InitiatePayment(reg.UserID, InitiatePaymentRequest{
		BeneficiaryName:    "Test",
		BeneficiaryAccount: "test@upi",
		AmountPaise:        100000,
		UserConsent:        true,
	})
	if err != nil {
		t.Fatalf("initiate payment failed: %v", err)
	}

	_, err = svc.VerifyPaymentReceipt(reg.UserID, pay.ID, VerifyReceiptRequest{
		ReceiptHash: "hash1",
		VerifiedBy:  "bank",
	})
	if err != nil {
		t.Fatalf("first verify failed: %v", err)
	}

	_, err = svc.VerifyPaymentReceipt(reg.UserID, pay.ID, VerifyReceiptRequest{
		ReceiptHash: "hash2",
		VerifiedBy:  "bank",
	})
	if err == nil {
		t.Fatal("expected error for duplicate receipt verification")
	}
}
