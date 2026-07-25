package bank

// Razorpay real bank provider — REAL-READY STUB.
//
// Satisfies platform.BankAdapter and is selectable by env (BANK_PROVIDER=razorpay),
// but every method returns ErrNotImplemented rather than faking success.
// Complete it by wiring Razorpay (test mode first, RAZORPAY_KEY):
//
//   VerifyUPI          POST https://api.razorpay.com/v1/payments/validate/vpa
//                      auth: Basic <key_id:key_secret>
//                      body: { "vpa": "user@bank" }
//                      -> { "success": true, "customer_name": "..." }
//
//   VerifyIFSC         GET  https://ifsc.razorpay.com/<IFSC>
//                      -> bank/branch; account-number validation via a
//                         penny-drop (Fund Account Validation) if needed.
//
//   GetBalance         Account Aggregator (Setu/Finvu) balance fetch, or the
//                      linked-account balance API — Razorpay itself is payout,
//                      not balance-of-record.
//
//   Connect            Create a RazorpayX contact + fund account for the user.
//
//   FetchTransactions  RazorpayX transactions API / webhook ingestion.
//
//   SubmitPayment      POST https://api.razorpay.com/v1/payouts
//                      body: { "fund_account_id", "amount", "currency", "mode" }
//                      -> { "id": "pout_...", "status": "processing|processed" }
//
// Read the base URL + key from env so no code change is needed to go live.

import (
	"context"
	"os"
	"time"

	"FINIX/backend/internal/domain/platform"
)

// RazorpayConfig holds the real-provider credentials (from env).
type RazorpayConfig struct {
	BaseURL string
	KeyID   string
}

// RazorpayConfigFromEnv reads RAZORPAY_BASE_URL / RAZORPAY_KEY.
func RazorpayConfigFromEnv() RazorpayConfig {
	base := os.Getenv("RAZORPAY_BASE_URL")
	if base == "" {
		base = "https://api.razorpay.com" // set RAZORPAY_BASE_URL for a sandbox
	}
	return RazorpayConfig{BaseURL: base, KeyID: os.Getenv("RAZORPAY_KEY")}
}

// Razorpay is the real bank provider (unwired stub).
type Razorpay struct {
	cfg RazorpayConfig
}

var _ platform.BankAdapter = (*Razorpay)(nil)

// NewRazorpay returns the real provider as the interface type.
func NewRazorpay(cfg RazorpayConfig) platform.BankAdapter { return &Razorpay{cfg: cfg} }

func (r *Razorpay) VerifyUPI(ctx context.Context, upiID string) (string, string, bool, error) {
	return "", "", false, ErrNotImplemented
}

func (r *Razorpay) VerifyIFSC(ctx context.Context, ifsc, accountNumber string) (string, string, bool, error) {
	return "", "", false, ErrNotImplemented
}

func (r *Razorpay) GetBalance(ctx context.Context, accountID string) (int64, error) {
	return 0, ErrNotImplemented
}

func (r *Razorpay) Connect(ctx context.Context, userID string, req platform.BankConnectionRequest) (platform.BankConnectionStatus, error) {
	return platform.BankConnectionStatus{}, ErrNotImplemented
}

func (r *Razorpay) FetchTransactions(ctx context.Context, userID string, since time.Time) ([]platform.BankTransactionEvent, error) {
	return nil, ErrNotImplemented
}

func (r *Razorpay) SubmitPayment(ctx context.Context, p platform.PaymentRequest) (string, error) {
	return "", ErrNotImplemented
}
