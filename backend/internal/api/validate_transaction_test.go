package api

import (
	"testing"

	"FINIX/backend/internal/domain/platform"
)

// TestValidateTransactionRequestCeiling verifies the per-transaction ceiling
// (₹10,00,000 = 100_000_000 paise). Amounts above it must be rejected — the
// handler maps this error to HTTP 400 Bad Request before the domain layer runs.
func TestValidateTransactionRequestCeiling(t *testing.T) {
	cases := []struct {
		name    string
		paise   int64
		wantErr bool
	}{
		{"zero rejected", 0, true},
		{"negative rejected", -100, true},
		{"one paise ok", 1, false},
		{"at ceiling ok", maxPaymentPaise, false},
		{"one over ceiling rejected", maxPaymentPaise + 1, true},
		{"far over ceiling rejected", 500_000_000, true}, // ₹50,00,000
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTransactionRequest(platform.InitiateTransactionRequest{
				AmountPaise: tc.paise,
				Recipient:   "someone@upi",
			})
			if tc.wantErr && err == nil {
				t.Fatalf("amount %d: expected rejection (→400), got nil", tc.paise)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("amount %d: expected accept, got error %v", tc.paise, err)
			}
		})
	}
}
