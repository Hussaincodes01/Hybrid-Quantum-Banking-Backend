// Package bank provides bank-integration providers for the FINIX platform.
//
// MockBank is the DEFAULT provider: an in-memory bank seeded with the demo
// accounts (previously hardcoded in platform/dummy_bank.go) so the offline demo
// keeps working, but now everything flows through the BankAdapter seam. A real
// provider (razorpay.go) can be swapped in by env with no call-site changes.
package bank

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"FINIX/backend/internal/domain/platform"
)

// account is the mock's internal record.
type account struct {
	AccountNumber string
	IFSC          string
	BankName      string
	Branch        string
	AccountType   string
	BalancePaise  int64
	UPIID         string
	HolderName    string
}

// MockBank is an offline, in-memory BankAdapter.
type MockBank struct {
	mu          sync.RWMutex
	byUPI       map[string]*account
	connections map[string]platform.BankConnectionStatus
	txns        map[string][]platform.BankTransactionEvent // keyed by UPI id / account

	// Demo realism knobs (optional; zero values = instant, no errors).
	latency  time.Duration
	failNext error
}

// Compile-time proof MockBank satisfies the adapter contract.
var _ platform.BankAdapter = (*MockBank)(nil)

// NewMockBank returns the default mock seeded with the demo accounts. It returns
// the interface type so callers depend on the seam, not the concrete mock.
func NewMockBank() platform.BankAdapter {
	m := &MockBank{
		byUPI:       make(map[string]*account),
		connections: make(map[string]platform.BankConnectionStatus),
		txns:        make(map[string][]platform.BankTransactionEvent),
	}
	m.seed()
	return m
}

// WithLatency injects a fixed per-call delay for demo realism.
func (m *MockBank) WithLatency(d time.Duration) *MockBank { m.latency = d; return m }

// seed loads the demo accounts (mirrors the retired dummy_bank.go data).
func (m *MockBank) seed() {
	seedData := []account{
		{"12345678901", "SBIN0001234", "State Bank of India", "MG Road, Bangalore", "savings", 35000000, "jiyad@sbi", "Jiyad"},
		{"98765432109", "ICIC0000456", "ICICI Bank", "Anna Salai, Chennai", "savings", 28000000, "venkat@icici", "Venkat"},
		{"56789012345", "KKBK0007789", "Kotak Mahindra Bank", "Bandra West, Mumbai", "savings", 60000000, "shubham@oksbi", "RD Shubham"},
		{"11112222333", "HDFC0004321", "HDFC Bank", "Corporate Banking", "current", 0, "electricity@hdfcbank", "BESCOM Electricity Board"},
		{"22223333444", "IOCL0000123", "Indian Oil Corporation", "LPG Division", "current", 0, "bharatgas@indianoil", "Bharat Gas Agency"},
		{"33334444555", "PYTM0001234", "Paytm Payments Bank", "Noida", "current", 0, "amazon@paytm", "Amazon India"},
		{"44445555666", "HDFC0000987", "HDFC Bank", "Brokerage Division", "current", 0, "zerodha@hdfc", "Zerodha Broking"},
		{"55556666777", "PUNB0001234", "Punjab National Bank", "Education Division", "current", 0, "school@pnb", "Delhi Public School"},
	}
	for i := range seedData {
		a := seedData[i]
		m.byUPI[a.UPIID] = &a
	}
}

func (m *MockBank) simulate(ctx context.Context) error {
	if m.latency > 0 {
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if m.failNext != nil {
		err := m.failNext
		m.failNext = nil
		return err
	}
	return nil
}

func (m *MockBank) VerifyUPI(ctx context.Context, upiID string) (string, string, bool, error) {
	if err := m.simulate(ctx); err != nil {
		return "", "", false, err
	}
	upiID = strings.ToLower(strings.TrimSpace(upiID))
	if upiID == "" {
		return "", "", false, fmt.Errorf("upi id is required")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if a, ok := m.byUPI[upiID]; ok {
		return "verified", a.HolderName, true, nil
	}
	return "not_found", "", false, nil
}

func (m *MockBank) VerifyIFSC(ctx context.Context, ifsc, accountNumber string) (string, string, bool, error) {
	if err := m.simulate(ctx); err != nil {
		return "", "", false, err
	}
	ifsc = strings.ToUpper(strings.TrimSpace(ifsc))
	accountNumber = strings.TrimSpace(accountNumber)
	if ifsc == "" || accountNumber == "" {
		return "", "", false, fmt.Errorf("ifsc and account number are required")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.byUPI {
		if a.IFSC == ifsc && a.AccountNumber == accountNumber {
			return "verified", a.HolderName, true, nil
		}
	}
	return "not_found", "", false, nil
}

func (m *MockBank) GetBalance(ctx context.Context, accountID string) (int64, error) {
	if err := m.simulate(ctx); err != nil {
		return 0, err
	}
	id := strings.ToLower(strings.TrimSpace(accountID))
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.byUPI {
		if strings.ToLower(a.AccountNumber) == id || a.UPIID == id {
			return a.BalancePaise, nil
		}
	}
	return 0, fmt.Errorf("account not found")
}

func (m *MockBank) Connect(ctx context.Context, userID string, req platform.BankConnectionRequest) (platform.BankConnectionStatus, error) {
	if err := m.simulate(ctx); err != nil {
		return platform.BankConnectionStatus{}, err
	}
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.connections[userID]
	if status.ConnectionID == "" {
		status.ConnectionID = "bankconn_" + randomHex(8)
		status.ConnectedAt = now
	}
	status.Provider = firstNonEmpty(req.Provider, "mock-bank")
	status.BaseURL = firstNonEmpty(req.BaseURL, "https://mock-bank.local")
	status.ClientID = firstNonEmpty(req.ClientID, "FINIX-mock")
	status.Connected = true
	status.Sandbox = req.Sandbox
	status.LastHeartbeatAt = now
	m.connections[userID] = status
	return status, nil
}

func (m *MockBank) FetchTransactions(ctx context.Context, userID string, since time.Time) ([]platform.BankTransactionEvent, error) {
	if err := m.simulate(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return any recorded events for the user plus a small seeded history.
	out := make([]platform.BankTransactionEvent, 0, 4)
	for _, ev := range m.txns[userID] {
		if ev.EventTimestamp.After(since) {
			out = append(out, ev)
		}
	}
	if len(out) == 0 {
		now := time.Now().UTC()
		seeded := []platform.BankTransactionEvent{
			{EventID: "mockevt_" + randomHex(6), BankCustomerID: userID, AmountPaise: 250000, Counterparty: "venkat@icici", Channel: "upi", EventTimestamp: now.Add(-24 * time.Hour), Source: "mock-bank"},
			{EventID: "mockevt_" + randomHex(6), BankCustomerID: userID, AmountPaise: 120000, Counterparty: "electricity@hdfcbank", Channel: "upi", EventTimestamp: now.Add(-72 * time.Hour), Source: "mock-bank"},
		}
		for _, ev := range seeded {
			if ev.EventTimestamp.After(since) {
				out = append(out, ev)
			}
		}
	}
	return out, nil
}

func (m *MockBank) SubmitPayment(ctx context.Context, p platform.PaymentRequest) (string, error) {
	if err := m.simulate(ctx); err != nil {
		return "", err
	}
	if p.AmountPaise <= 0 {
		return "", fmt.Errorf("amount must be positive")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Debit the payer if we know the account (demo balances).
	payer := strings.ToLower(strings.TrimSpace(p.FromAccount))
	if payer != "" {
		for _, a := range m.byUPI {
			if strings.ToLower(a.AccountNumber) == payer || a.UPIID == payer {
				if a.BalancePaise < p.AmountPaise {
					return "", fmt.Errorf("insufficient balance")
				}
				a.BalancePaise -= p.AmountPaise
				break
			}
		}
	}

	ref := "MOCKPAY" + strings.ToUpper(randomHex(8))
	// Record an event so FetchTransactions/webhook ingestion can see it.
	m.txns[p.UserID] = append(m.txns[p.UserID], platform.BankTransactionEvent{
		EventID:        ref,
		BankCustomerID: p.UserID,
		AmountPaise:    p.AmountPaise,
		Counterparty:   firstNonEmpty(p.Beneficiary, p.BeneficiaryName),
		Channel:        firstNonEmpty(p.Method, "upi"),
		EventTimestamp: time.Now().UTC(),
		Source:         "mock-bank",
	})
	return ref, nil
}
