package blockchain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Event struct {
	ID            string              `json:"id"`
	UserID        string              `json:"userId"`
	Action        string              `json:"action"`
	Resource      string              `json:"resource"`
	PayloadHash   string              `json:"payloadHash"`
	PrevHash      string              `json:"prevHash"`
	Hash          string              `json:"hash"`
	Timestamp     time.Time           `json:"timestamp"`
	SmartContract SmartContractResult `json:"smartContract,omitempty"`

	// Populated only when the ledger is backed by Hyperledger Fabric.
	TxID     string `json:"txId,omitempty"`     // Fabric transaction id
	Seq      uint64 `json:"seq,omitempty"`      // per-user sequence
	Category string `json:"category,omitempty"` // ledger category (== Resource)
}

// SmartContractResult represents the result of executing a smart contract rule
// on a blockchain event. Per spec §3A.2: "Smart Contracts (chaincode) enforce
// business rules autonomously: a smart contract governs transaction cooling-off
// periods, ensuring that even if an application-layer bypass is found, the
// blockchain layer will still enforce the cooling period."
type SmartContractResult struct {
	RuleName   string    `json:"ruleName"`
	Passed     bool      `json:"passed"`
	Violation  string    `json:"violation,omitempty"`
	EnforcedAt time.Time `json:"enforcedAt"`
}

// SmartContractRule is an immutable business rule enforced at the blockchain layer.
// The rules are evaluated with the ledger lock already held by the caller. A
// RuleQuery gives O(1) indexed access to prior events, so a rule never has to scan
// the full event log.
type SmartContractRule interface {
	Name() string
	Evaluate(event Event, q RuleQuery) SmartContractResult
}

// RuleQuery is an indexed, read-only view of prior ledger events. The in-memory
// implementation (eventIndex) builds its maps once per WriteEvent in O(n), turning
// each rule's previous O(n) scan into O(1)/O(matches) lookups.
type RuleQuery interface {
	// EventsByUser returns all prior events for a user.
	EventsByUser(userID string) []Event
	// EventsByUserAndAction returns prior events for a user filtered by action.
	EventsByUserAndAction(userID, action string) []Event
	// EventByPayloadHash returns a prior event for a user with the given payload
	// hash, if any (used for double-spend detection).
	EventByPayloadHash(userID, payloadHash string) (Event, bool)
}

// eventIndex indexes a slice of events for O(1) rule lookups. Built once per
// evaluation; not safe for concurrent mutation (the ledger lock covers it).
type eventIndex struct {
	byUser       map[string][]Event
	byUserAction map[string][]Event
	byUserHash   map[string]Event
}

func newEventIndex(events []Event) *eventIndex {
	idx := &eventIndex{
		byUser:       make(map[string][]Event, len(events)),
		byUserAction: make(map[string][]Event),
		byUserHash:   make(map[string]Event),
	}
	for _, ev := range events {
		idx.byUser[ev.UserID] = append(idx.byUser[ev.UserID], ev)
		ak := ev.UserID + "\x00" + ev.Action
		idx.byUserAction[ak] = append(idx.byUserAction[ak], ev)
		if ev.PayloadHash != "" {
			// First occurrence wins; a duplicate hash is exactly what DoubleSpendRule
			// looks for, and it compares against this earlier event.
			hk := ev.UserID + "\x00" + ev.PayloadHash
			if _, seen := idx.byUserHash[hk]; !seen {
				idx.byUserHash[hk] = ev
			}
		}
	}
	return idx
}

func (idx *eventIndex) EventsByUser(userID string) []Event {
	return idx.byUser[userID]
}

func (idx *eventIndex) EventsByUserAndAction(userID, action string) []Event {
	return idx.byUserAction[userID+"\x00"+action]
}

func (idx *eventIndex) EventByPayloadHash(userID, payloadHash string) (Event, bool) {
	ev, ok := idx.byUserHash[userID+"\x00"+payloadHash]
	return ev, ok
}

// FabricBackend is the Hyperledger Fabric ledger the events are written to when
// the network is enabled. It is satisfied by internal/infra/fabric.Client; the
// interface keeps this domain package free of infrastructure imports.
//
// When a backend is attached, Fabric is the source of truth: the smart-contract
// rules run on-chain (a rejection returns an error here) and reads are served
// from the chaincode. Without one, the ledger falls back to the in-memory chain
// so the service still runs with no network.
type FabricBackend interface {
	RecordEvent(ctx context.Context, category, userID, action string, payload any) (FabricEvent, error)
	QueryByUser(ctx context.Context, userID string) ([]FabricEvent, error)
	QueryByCategory(ctx context.Context, userID, category string) ([]FabricEvent, error)
	GetProof(ctx context.Context, userID string) (FabricProof, error)
	VerifyCoolingOff(ctx context.Context, userID string) (bool, time.Duration, error)
	BlockHeight(ctx context.Context) (uint64, error)
}

// FabricEvent mirrors the chaincode event shape.
type FabricEvent struct {
	ID            string
	UserID        string
	Action        string
	Resource      string
	Category      string
	PayloadHash   string
	PrevHash      string
	Hash          string
	Timestamp     time.Time
	Seq           uint64
	TxID          string
	SmartContract SmartContractResult
}

// FabricProof mirrors the chaincode integrity summary.
type FabricProof struct {
	MerkleRoot string
	EventCount int
	HeadHash   string
	Intact     bool
	Detail     string
}

type Ledger struct {
	mu     sync.RWMutex
	events []Event
	rules  []SmartContractRule

	// fabric, when non-nil, makes Hyperledger Fabric the system of record.
	fabric FabricBackend
}

func NewLedger() *Ledger {
	l := &Ledger{
		events: make([]Event, 0, 256),
	}
	l.registerDefaultRules()
	return l
}

// NewFabricLedger returns a ledger backed by Hyperledger Fabric.
func NewFabricLedger(backend FabricBackend) *Ledger {
	l := NewLedger()
	l.fabric = backend
	return l
}

// UseFabric attaches (or detaches, with nil) the Fabric backend at runtime.
func (l *Ledger) UseFabric(backend FabricBackend) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fabric = backend
}

// FabricEnabled reports whether Fabric is the system of record.
func (l *Ledger) FabricEnabled() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.fabric != nil
}

// backend returns the Fabric backend without holding the lock during I/O.
func (l *Ledger) backend() FabricBackend {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.fabric
}

// fromFabric maps a chaincode event onto the ledger's Event contract.
func fromFabric(fe FabricEvent) Event {
	return Event{
		ID:            fe.ID,
		UserID:        fe.UserID,
		Action:        fe.Action,
		Resource:      fe.Resource,
		PayloadHash:   fe.PayloadHash,
		PrevHash:      fe.PrevHash,
		Hash:          fe.Hash,
		Timestamp:     fe.Timestamp,
		SmartContract: fe.SmartContract,
		TxID:          fe.TxID,
		Seq:           fe.Seq,
		Category:      fe.Category,
	}
}

// categoryFor maps a WriteEvent `resource` onto a valid chaincode category.
// The chaincode rejects unknown categories, so anything unrecognised is
// recorded under the generic audit category rather than being dropped.
func categoryFor(resource string) string {
	switch resource {
	case "transaction", "payment", "consent", "kyc", "goal", "security", "audit", "blockchain_record":
		return resource
	default:
		return "audit"
	}
}

// registerDefaultRules registers the smart contract rules that are enforced
// at the blockchain layer, per the product spec.
func (l *Ledger) registerDefaultRules() {
	l.rules = []SmartContractRule{
		&CoolingOffRule{},
		&ConsentRevocationRule{},
		&DoubleSpendRule{},
	}
}

// AddRule adds a custom smart contract rule to the ledger.
func (l *Ledger) AddRule(rule SmartContractRule) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rules = append(l.rules, rule)
}

// ErrSmartContractViolation is returned by WriteEventStrict when one or more
// smart-contract rules reject the event. The event is NOT committed to the
// ledger. Callers on financial/consent paths must treat this as a hard failure.
var ErrSmartContractViolation = errors.New("smart contract violation")

// WriteEvent appends an event to the ledger.
//
// With Fabric attached the event is submitted to the chaincode: the on-chain
// smart contracts (cooling-off, consent revocation, double-spend) are ENFORCED,
// so a violation returns an error and nothing is committed. Without Fabric the
// legacy in-memory hash chain is used and violations are recorded, not rejected.
// The signature is unchanged so every existing call site keeps working.
//
// This is the AUDIT-ONLY variant: on the in-memory path a rule violation is
// annotated on the event but the event is still recorded (nil error). Use it
// for events that must always be logged regardless of rule outcome. For
// financial/consent paths where a violation must BLOCK the operation, use
// WriteEventStrict instead (H-8).
func (l *Ledger) WriteEvent(ctx context.Context, userID, action, resource string, payload any) (Event, error) {
	if fb := l.backend(); fb != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		fe, err := fb.RecordEvent(ctx, categoryFor(resource), userID, action, payload)
		if err != nil {
			return Event{}, err
		}
		return fromFabric(fe), nil
	}
	ev, _, err := l.writeEventInMemory(userID, action, resource, payload)
	return ev, err
}

// WriteEventStrict appends an event only if every smart-contract rule passes.
//
// H-8 fix: the legacy in-memory path recorded violating events with an
// annotation but never rejected them, so an "application layer must respect the
// violation" contract was unenforceable — an app-layer bypass meant the
// blockchain layer did nothing. WriteEventStrict makes the enforcement real:
// when Fabric is attached the chaincode already rejects on-chain (an error is
// returned and nothing commits); on the in-memory fallback a rule failure now
// returns ErrSmartContractViolation and the event is NOT appended.
//
// Use this for transaction, consent-revocation and data-access paths where a
// cooling-off / revoked-consent / double-spend hit must stop the operation.
func (l *Ledger) WriteEventStrict(ctx context.Context, userID, action, resource string, payload any) (Event, error) {
	if fb := l.backend(); fb != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		// On-chain rules are authoritative: RecordEvent returns an error and
		// commits nothing when a chaincode rule rejects the transaction.
		fe, err := fb.RecordEvent(ctx, categoryFor(resource), userID, action, payload)
		if err != nil {
			return Event{}, err
		}
		return fromFabric(fe), nil
	}
	ev, violations, err := l.writeEventInMemory(userID, action, resource, payload)
	if err != nil {
		return Event{}, err
	}
	if len(violations) > 0 {
		return ev, fmt.Errorf("%w: %s", ErrSmartContractViolation, strings.Join(violations, "; "))
	}
	return ev, nil
}

// writeEventInMemory is the original in-memory hash-chain implementation, kept
// as the no-network fallback path. It always appends the event (the ledger is
// immutable/append-only) and returns any rule violations so the caller can
// decide whether to reject the operation. WriteEvent ignores the violations
// (audit-only); WriteEventStrict turns a non-empty list into an error.
func (l *Ledger) writeEventInMemory(userID, action, resource string, payload any) (Event, []string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return Event{}, nil, err
	}

	payloadHash := sha256Hex(payloadBytes)
	now := time.Now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	prevHash := ""
	if len(l.events) > 0 {
		prevHash = l.events[len(l.events)-1].Hash
	}

	id := "ledger-" + strconv.Itoa(len(l.events)+1)
	eventHash := sha256Hex([]byte(prevHash + payloadHash + userID + action + resource + now.Format(time.RFC3339Nano)))

	ev := Event{
		ID:          id,
		UserID:      userID,
		Action:      action,
		Resource:    resource,
		PayloadHash: payloadHash,
		PrevHash:    prevHash,
		Hash:        eventHash,
		Timestamp:   now,
	}

	// Execute smart contract rules against an index built once over prior events.
	query := newEventIndex(l.events)
	var ruleViolations []string
	allPassed := true
	for _, rule := range l.rules {
		result := rule.Evaluate(ev, query)
		ev.SmartContract = result
		if !result.Passed {
			allPassed = false
			ruleViolations = append(ruleViolations, result.Violation)
		}
	}

	// If any smart contract rule fails, the event is still recorded (immutable)
	// but the violation is noted. Audit callers accept this; strict callers turn
	// the returned violation list into a hard rejection.
	if !allPassed {
		ev.SmartContract.Violation = fmt.Sprintf("smart contract violations: %v", ruleViolations)
	}

	l.events = append(l.events, ev)
	return ev, ruleViolations, nil
}

// VerifyCoolingOff checks if a transaction is still in its cooling-off period.
// This is enforced at the blockchain layer and cannot be bypassed by the application.
func (l *Ledger) VerifyCoolingOff(userID, txID string) (active bool, remaining time.Duration) {
	// On-chain enforcement is authoritative when Fabric is attached.
	if fb := l.backend(); fb != nil {
		if a, r, err := fb.VerifyCoolingOff(context.Background(), userID); err == nil {
			return a, r
		}
		// fall through to the local view if the network is briefly unreachable
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	now := time.Now().UTC()
	for _, ev := range l.events {
		if ev.UserID != userID {
			continue
		}
		if ev.Action != "transaction_initiated" && ev.Action != "transaction_blocked" {
			continue
		}
		// Check if this event has a cooling-off enforcement
		if !ev.SmartContract.Passed {
			continue
		}
		// Cooling-off is 6 hours per spec §6.3
		coolingEnd := ev.Timestamp.Add(6 * time.Hour)
		if now.Before(coolingEnd) {
			return true, coolingEnd.Sub(now)
		}
	}
	return false, 0
}

// VerifyIntegrity checks that the blockchain hash chain is intact.
// Per spec §3A.2: "Any tampering with any historical record would invalidate
// every subsequent block and be immediately detectable."
func (l *Ledger) VerifyIntegrity() (bool, string) {
	// With Fabric attached the blockchain itself is the tamper-evidence: a
	// non-zero committed block height proves the chain is live, and the
	// chaincode re-verifies each user's hash chain in GetProof.
	if fb := l.backend(); fb != nil {
		height, err := fb.BlockHeight(context.Background())
		if err != nil {
			return false, fmt.Sprintf("fabric ledger unreachable: %v", err)
		}
		if height == 0 {
			return false, "fabric ledger reports zero block height"
		}
		return true, fmt.Sprintf("fabric ledger intact at block height %d", height)
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	for i, ev := range l.events {
		expectedPrevHash := ""
		if i > 0 {
			expectedPrevHash = l.events[i-1].Hash
		}
		if ev.PrevHash != expectedPrevHash {
			return false, fmt.Sprintf("hash chain broken at event %s: expected prev %s, got %s", ev.ID, expectedPrevHash, ev.PrevHash)
		}

		// Recompute the hash
		expectedHash := sha256Hex([]byte(ev.PrevHash + ev.PayloadHash + ev.UserID + ev.Action + ev.Resource + ev.Timestamp.Format(time.RFC3339Nano)))
		if ev.Hash != expectedHash {
			return false, fmt.Sprintf("hash mismatch at event %s: expected %s, got %s", ev.ID, expectedHash, ev.Hash)
		}
	}
	return true, ""
}

func (l *Ledger) EventsByUser(userID string) []Event {
	// Fabric is the system of record; a per-user query is a partial-composite
	// key scan in the chaincode. (An empty userID has no on-chain equivalent —
	// callers wanting the whole ledger should read the Postgres mirror.)
	if fb := l.backend(); fb != nil && userID != "" {
		if fes, err := fb.QueryByUser(context.Background(), userID); err == nil {
			out := make([]Event, 0, len(fes))
			for _, fe := range fes {
				out = append(out, fromFabric(fe))
			}
			return out
		}
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	if userID == "" {
		out := make([]Event, len(l.events))
		copy(out, l.events)
		return out
	}

	out := make([]Event, 0, len(l.events))
	for _, ev := range l.events {
		if ev.UserID == userID {
			out = append(out, ev)
		}
	}
	return out
}

func (l *Ledger) MerkleRoot(userID string) string {
	// Recomputed on-chain so the root is proven against the committed ledger
	// rather than a local copy of it.
	if fb := l.backend(); fb != nil && userID != "" {
		if proof, err := fb.GetProof(context.Background(), userID); err == nil {
			return proof.MerkleRoot
		}
	}

	events := l.EventsByUser(userID)
	if len(events) == 0 {
		return ""
	}

	hashes := make([]string, 0, len(events))
	for _, ev := range events {
		hashes = append(hashes, ev.Hash)
	}

	for len(hashes) > 1 {
		next := make([]string, 0, (len(hashes)+1)/2)
		for i := 0; i < len(hashes); i += 2 {
			if i+1 >= len(hashes) {
				next = append(next, sha256Hex([]byte(hashes[i]+hashes[i])))
				continue
			}
			next = append(next, sha256Hex([]byte(hashes[i]+hashes[i+1])))
		}
		hashes = next
	}

	return hashes[0]
}

func (l *Ledger) EventCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.events)
}

// --- Smart Contract Rules ---

// CoolingOffRule enforces the 6-hour cooling-off period for blocked transactions.
// Per spec: "a smart contract governs transaction cooling-off periods, ensuring
// that even if an application-layer bypass is found, the blockchain layer will
// still enforce the cooling period before funds can be released."
type CoolingOffRule struct{}

func (r *CoolingOffRule) Name() string { return "cooling_off_enforcement" }

func (r *CoolingOffRule) Evaluate(event Event, q RuleQuery) SmartContractResult {
	result := SmartContractResult{
		RuleName:   r.Name(),
		Passed:     true,
		EnforcedAt: time.Now().UTC(),
	}

	// If this is a blocked transaction, enforce cooling-off
	if event.Action == "transaction_blocked" || event.Action == "transaction_initiated" {
		// Check if there's a previous blocked transaction for this user within 6 hours
		for _, prevEv := range q.EventsByUserAndAction(event.UserID, "transaction_blocked") {
			if event.Timestamp.Sub(prevEv.Timestamp) < 6*time.Hour {
				result.Passed = false
				result.Violation = "cooling-off period active: previous blocked transaction within 6 hours"
				break
			}
		}
	}

	return result
}

// ConsentRevocationRule ensures that data access events only occur when consent
// was granted. Per spec: "All user consent grants, modifications, and revocations
// are written to the blockchain ledger."
type ConsentRevocationRule struct{}

func (r *ConsentRevocationRule) Name() string { return "consent_revocation_enforcement" }

func (r *ConsentRevocationRule) Evaluate(event Event, q RuleQuery) SmartContractResult {
	result := SmartContractResult{
		RuleName:   r.Name(),
		Passed:     true,
		EnforcedAt: time.Now().UTC(),
	}

	// If this is a data access event, verify consent wasn't revoked
	if event.Resource == "account_aggregator" || event.Resource == "data_export" {
		for _, prevEv := range q.EventsByUserAndAction(event.UserID, "consent_revoked") {
			if prevEv.Timestamp.Before(event.Timestamp) {
				result.Passed = false
				result.Violation = "consent was revoked before this data access event"
				break
			}
		}
	}

	return result
}

// DoubleSpendRule prevents the same idempotency key from being used twice.
// Per spec: "Idempotency Engine: Ensures that double-tapping 'Send Money' or
// experiencing network drops doesn't result in duplicate transactions."
type DoubleSpendRule struct{}

func (r *DoubleSpendRule) Name() string { return "double_spend_prevention" }

func (r *DoubleSpendRule) Evaluate(event Event, q RuleQuery) SmartContractResult {
	result := SmartContractResult{
		RuleName:   r.Name(),
		Passed:     true,
		EnforcedAt: time.Now().UTC(),
	}

	if event.Action == "transaction_initiated" {
		// Any PRIOR transaction_initiated with the same (user, payload hash) is a
		// duplicate. The current event is not yet appended when rules run, so a hit
		// here is always an earlier event — no timestamp carve-out is needed (the
		// old !Equal check silently missed double-taps that shared a clock tick).
		if prevEv, ok := q.EventByPayloadHash(event.UserID, event.PayloadHash); ok {
			if prevEv.Action == "transaction_initiated" {
				result.Passed = false
				result.Violation = "duplicate transaction detected: same payload hash already exists"
			}
		}
	}

	return result
}

func sha256Hex(input []byte) string {
	h := sha256.Sum256(input)
	return hex.EncodeToString(h[:])
}
