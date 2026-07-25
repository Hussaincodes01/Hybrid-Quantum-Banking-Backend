package platform

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// LedgerEntryType classifies the financial nature of each entry.
type LedgerEntryType string

const (
	LedgerDebit  LedgerEntryType = "debit"
	LedgerCredit LedgerEntryType = "credit"
)

// AccountingLedgerEntry is an immutable double-entry record.
// Every financial event produces exactly two entries (one debit, one credit)
// where debits MUST equal credits. This is the fundamental accounting invariant
// required by banking regulations (RBI, PCI-DSS, IFRS).
type AccountingLedgerEntry struct {
	ID              string          `json:"id"`
	TransactionRef  string          `json:"transactionRef"`
	AccountID       string          `json:"accountId"`
	UserID          string          `json:"userId"`
	EntryType       LedgerEntryType `json:"entryType"`
	AmountPaise     int64           `json:"amountPaise"`
	CounterpartyRef string          `json:"counterpartyRef"`
	Narration       string          `json:"narration"`
	CreatedAt       time.Time       `json:"createdAt"`
}

// AccountingLedger is the double-entry accounting book.
// It enforces: sum(debits) == sum(credits) for every transaction batch.
// All validations are server-side only — the frontend never computes balances.
type AccountingLedger struct {
	mu              sync.RWMutex
	entries         []AccountingLedgerEntry
	suspenseAccount string // "acct-suspense" — holds unmatched entries for reconciliation
}

func NewAccountingLedger() *AccountingLedger {
	return &AccountingLedger{
		entries:         make([]AccountingLedgerEntry, 0, 10240),
		suspenseAccount: "acct-suspense",
	}
}

// PostTransaction records a double-entry pair for a financial transaction.
// Debit: source account (money leaves)
// Credit: destination/revenue account (money arrives)
// Both entries share the same transactionRef for audit traceability.
// Server-side invariant enforcement: cannot post if amount <= 0 or accounts are empty.
func (al *AccountingLedger) PostTransaction(
	transactionRef, debitAccountID, debitUserID, creditAccountID, creditUserID string,
	amountPaise int64, narration string,
) (debitEntry, creditEntry AccountingLedgerEntry, err error) {
	if amountPaise <= 0 {
		return AccountingLedgerEntry{}, AccountingLedgerEntry{},
			fmt.Errorf("ledger: amount must be positive, got %d", amountPaise)
	}
	if debitAccountID == "" || creditAccountID == "" {
		return AccountingLedgerEntry{}, AccountingLedgerEntry{},
			errors.New("ledger: both debit and credit account IDs are required")
	}
	if debitAccountID == creditAccountID {
		return AccountingLedgerEntry{}, AccountingLedgerEntry{},
			errors.New("ledger: cannot debit and credit the same account")
	}

	now := time.Now().UTC()
	debitEntry = AccountingLedgerEntry{
		ID:              "ldr_" + randomHex(12),
		TransactionRef:  transactionRef,
		AccountID:       debitAccountID,
		UserID:          debitUserID,
		EntryType:       LedgerDebit,
		AmountPaise:     amountPaise,
		CounterpartyRef: creditAccountID,
		Narration:       narration,
		CreatedAt:       now,
	}
	creditEntry = AccountingLedgerEntry{
		ID:              "ldr_" + randomHex(12),
		TransactionRef:  transactionRef,
		AccountID:       creditAccountID,
		UserID:          creditUserID,
		EntryType:       LedgerCredit,
		AmountPaise:     amountPaise,
		CounterpartyRef: debitAccountID,
		Narration:       narration,
		CreatedAt:       now,
	}

	al.mu.Lock()
	al.entries = append(al.entries, debitEntry, creditEntry)
	al.mu.Unlock()

	return debitEntry, creditEntry, nil
}

// PostOutgoingPayment records a payment from a FINIX user to an external recipient.
// Debit: user's account (reduces balance)
// Credit: external recipient (revenue/suspense tracking)
func (al *AccountingLedger) PostOutgoingPayment(
	transactionRef, userAccountID, userID, recipientRef string, amountPaise int64, narration string,
) (debitEntry, creditEntry AccountingLedgerEntry, err error) {
	return al.PostTransaction(
		transactionRef,
		userAccountID, userID, // debit: user's account
		recipientRef, "", // credit: external recipient (no FINIX user)
		amountPaise, narration,
	)
}

// PostContribution records a goal contribution (internal transfer).
// Debit: user's bank account
// Credit: goal savings account (virtual, tracked in goal.SavedAmountPaise)
func (al *AccountingLedger) PostContribution(
	transactionRef, userAccountID, userID string, amountPaise int64, goalName string,
) (debitEntry, creditEntry AccountingLedgerEntry, err error) {
	return al.PostTransaction(
		transactionRef,
		userAccountID, userID,
		"goals-virtual", userID,
		amountPaise,
		fmt.Sprintf("Goal contribution: %s", goalName),
	)
}

// PostRevenue records income (salary, dividends, interest) into a user's account.
// Debit: revenue source (external)
// Credit: user's account (increases balance)
func (al *AccountingLedger) PostRevenue(
	transactionRef, creditAccountID, userID, sourceRef string, amountPaise int64, narration string,
) (debitEntry, creditEntry AccountingLedgerEntry, err error) {
	return al.PostTransaction(
		transactionRef,
		sourceRef, "", // debit: external revenue source
		creditAccountID, userID, // credit: user's account
		amountPaise, narration,
	)
}

// GetBalance computes the current balance for an account by summing all entries.
// Balance = sum(credits) - sum(debits). This is THE source of truth for balances.
// The frontend NEVER computes balances — it always calls this server-side method.
func (al *AccountingLedger) GetBalance(accountID string) int64 {
	al.mu.RLock()
	defer al.mu.RUnlock()
	var balance int64
	for _, e := range al.entries {
		if e.AccountID != accountID {
			continue
		}
		switch e.EntryType {
		case LedgerCredit:
			balance += e.AmountPaise
		case LedgerDebit:
			balance -= e.AmountPaise
		}
	}
	return balance
}

// GetEntriesByUser returns all ledger entries for a user's accounts.
func (al *AccountingLedger) GetEntriesByUser(userID string, limit int) []AccountingLedgerEntry {
	al.mu.RLock()
	defer al.mu.RUnlock()
	result := make([]AccountingLedgerEntry, 0)
	for i := len(al.entries) - 1; i >= 0 && len(result) < limit; i-- {
		if al.entries[i].UserID == userID {
			result = append(result, al.entries[i])
		}
	}
	return result
}

// GetEntriesByTransaction returns all ledger entries for a specific transaction.
func (al *AccountingLedger) GetEntriesByTransaction(transactionRef string) []AccountingLedgerEntry {
	al.mu.RLock()
	defer al.mu.RUnlock()
	result := make([]AccountingLedgerEntry, 0)
	for _, e := range al.entries {
		if e.TransactionRef == transactionRef {
			result = append(result, e)
		}
	}
	return result
}

// VerifyBalance sanity-checks that sum(all debits) == sum(all credits) across the entire ledger.
// This is the fundamental banking invariant. Must be called during reconciliation.
func (al *AccountingLedger) VerifyBalance() (debits, credits int64, balanced bool) {
	al.mu.RLock()
	defer al.mu.RUnlock()
	for _, e := range al.entries {
		switch e.EntryType {
		case LedgerDebit:
			debits += e.AmountPaise
		case LedgerCredit:
			credits += e.AmountPaise
		}
	}
	return debits, credits, debits == credits
}

// EntryCount returns the total number of ledger entries.
func (al *AccountingLedger) EntryCount() int {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return len(al.entries)
}

// AllEntries returns all ledger entries for persistence/reconciliation.
func (al *AccountingLedger) AllEntries() []AccountingLedgerEntry {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return append([]AccountingLedgerEntry(nil), al.entries...)
}
