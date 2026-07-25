package bank

import (
	"context"
	"errors"
	"testing"

	"FINIX/backend/internal/domain/platform"
)

func buildPayment(from, to string, amountPaise int64) platform.PaymentRequest {
	return platform.PaymentRequest{
		UserID:      "usr_test",
		FromAccount: from,
		Beneficiary: to,
		AmountPaise: amountPaise,
		Currency:    "INR",
		Method:      "upi",
	}
}

func TestMockBankVerifiesSeededUPIs(t *testing.T) {
	m := NewMockBank()
	ctx := context.Background()

	// Seeded UPI resolves with the right holder + balance.
	status, holder, ok, err := m.VerifyUPI(ctx, "jiyad@sbi")
	if err != nil || !ok || status != "verified" || holder != "Jiyad" {
		t.Fatalf("jiyad@sbi: status=%q holder=%q ok=%v err=%v", status, holder, ok, err)
	}
	bal, err := m.GetBalance(ctx, "jiyad@sbi")
	if err != nil || bal != 35000000 {
		t.Fatalf("jiyad balance: got %d err=%v (want 35000000)", bal, err)
	}

	// Unknown UPI is not_found (not an error).
	status, _, ok, err = m.VerifyUPI(ctx, "nobody@nowhere")
	if err != nil || ok || status != "not_found" {
		t.Fatalf("unknown upi: status=%q ok=%v err=%v", status, ok, err)
	}
}

func TestMockBankSubmitPaymentDebits(t *testing.T) {
	m := NewMockBank()
	ctx := context.Background()

	before, _ := m.GetBalance(ctx, "venkat@icici")
	ref, err := m.SubmitPayment(ctx, buildPayment("venkat@icici", "jiyad@sbi", 500000))
	if err != nil || ref == "" {
		t.Fatalf("submit payment: ref=%q err=%v", ref, err)
	}
	after, _ := m.GetBalance(ctx, "venkat@icici")
	if after != before-500000 {
		t.Fatalf("balance not debited: before=%d after=%d", before, after)
	}

	// Overdraw is rejected.
	if _, err := m.SubmitPayment(ctx, buildPayment("venkat@icici", "jiyad@sbi", 1<<62)); err == nil {
		t.Fatal("expected insufficient-balance error on overdraw")
	}
}

func TestRazorpayStubIsNotImplemented(t *testing.T) {
	r := NewRazorpay(RazorpayConfigFromEnv())
	ctx := context.Background()

	// Every method must return the not-implemented sentinel, never a fake success.
	if _, _, ok, err := r.VerifyUPI(ctx, "jiyad@sbi"); ok || !errors.Is(err, ErrNotImplemented) {
		t.Errorf("VerifyUPI: ok=%v err=%v (want ErrNotImplemented)", ok, err)
	}
	if _, err := r.GetBalance(ctx, "jiyad@sbi"); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("GetBalance err=%v (want ErrNotImplemented)", err)
	}
	if _, err := r.SubmitPayment(ctx, buildPayment("a", "b", 1)); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("SubmitPayment err=%v (want ErrNotImplemented)", err)
	}
}
