package platform

import (
	"context"
	"time"

	"FINIX/backend/internal/domain/security"
)

// BankAdapter is a swappable bank-integration provider. It is modelled on
// blockchain.FabricBackend: the interface lives in the domain, a local mock is
// the default (internal/infra/bank), and a real provider (Razorpay/AA) can be
// selected by env later — without touching any call site.
//
// The interface is deliberately provider-agnostic: no Razorpay/AA-specific
// fields leak in. Reuses the existing platform shapes (BankConnectionRequest,
// BankConnectionStatus, BankTransactionEvent from aiml.go) so callers already
// speaking those types don't change.
type BankAdapter interface {
	// VerifyUPI resolves a UPI id to its account holder. ok=false means the id
	// is unknown (status "not_found"), not an error.
	VerifyUPI(ctx context.Context, upiID string) (status string, holder string, ok bool, err error)

	// VerifyIFSC resolves an IFSC + account number to its holder.
	VerifyIFSC(ctx context.Context, ifsc, accountNumber string) (status string, holder string, ok bool, err error)

	// GetBalance returns the available balance in paise for an account
	// (identified by account number or UPI id).
	GetBalance(ctx context.Context, accountID string) (paise int64, err error)

	// Connect establishes/refreshes a bank-API connection for a user.
	Connect(ctx context.Context, userID string, req BankConnectionRequest) (BankConnectionStatus, error)

	// FetchTransactions pulls the user's bank transactions since a timestamp.
	FetchTransactions(ctx context.Context, userID string, since time.Time) ([]BankTransactionEvent, error)

	// SubmitPayment executes a payment and returns the bank reference id.
	SubmitPayment(ctx context.Context, p PaymentRequest) (ref string, err error)
}

// PaymentRequest is the provider-agnostic instruction to move money. It is
// distinct from the API-layer InitiatePaymentRequest so the adapter contract is
// not coupled to the HTTP shape.
type PaymentRequest struct {
	UserID          string
	FromAccount     string // payer account number or UPI id
	Beneficiary     string // payee UPI id or account number
	BeneficiaryName string
	AmountPaise     int64
	Currency        string
	Method          string // upi / imps / neft
	Reference       string // idempotency / client reference
}

// UseBankAdapter injects the bank provider. Mirrors UseFabricLedger. Passing nil
// is a no-op, so a call site can fall back to legacy behaviour when unset.
//
// Providers are injected once at startup (composition root) before any request
// is served, so this — and the read helper below — deliberately take no lock:
// most bank call sites already hold s.mu, and sync.RWMutex is not reentrant.
func (s *Service) UseBankAdapter(adapter BankAdapter) {
	if s == nil || adapter == nil {
		return
	}
	s.bank = adapter
}

// bankAdapter returns the injected provider (nil if none).
func (s *Service) bankAdapter() BankAdapter { return s.bank }

// UseKYCProvider injects the eKYC provider (mock by default, real by env).
func (s *Service) UseKYCProvider(p security.KYCProvider) {
	if s == nil || p == nil {
		return
	}
	s.kyc = p
}

// UseSIMVerifier injects the SIM-binding / VNN provider.
func (s *Service) UseSIMVerifier(v security.SIMVerifier) {
	if s == nil || v == nil {
		return
	}
	s.sim = v
}

// kycProvider / simVerifier return the injected providers (nil if none).
func (s *Service) kycProvider() security.KYCProvider { return s.kyc }
func (s *Service) simVerifier() security.SIMVerifier { return s.sim }

// UseJWTManager makes LoginWithPIN issue signed JWTs (spec §3.6) instead of
// opaque tokens. nil leaves the legacy opaque-token flow in place.
func (s *Service) UseJWTManager(m *security.JWTManager) {
	if s == nil {
		return
	}
	s.jwt = m
}

// JWTManager returns the injected JWT manager (nil if none).
func (s *Service) JWTManager() *security.JWTManager { return s.jwt }
