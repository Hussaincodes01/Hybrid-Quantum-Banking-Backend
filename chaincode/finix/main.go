// FINIX chaincode — the authoritative event ledger for the FINIX platform.
//
// Replaces the in-memory blockchain.Ledger in the backend. Every platform event
// (transaction, payment, consent, kyc, goal, security, audit, blockchain_record)
// is written here as an immutable, category-indexed ledger entry.
//
// DETERMINISM: chaincode runs on every endorsing peer and the results must match
// byte-for-byte. Never use time.Now() or rand here — the transaction timestamp
// (stub.GetTxTimestamp) and transaction id (stub.GetTxID) are the only permitted
// sources of "now" and identity.
//
// KEY LAYOUT: composite key evt~{userID}~{category}~{seq}. This ordering makes
// both primary reads a partial-composite range scan:
//   QueryByUser(user)                -> partial [user]
//   QueryByCategory(user, category)  -> partial [user, category]
//
// HASH CHAIN: a per-user chain (head key hd~{userID}) links each event to the
// previous one via PrevHash, preserving the backend's Event contract. Fabric's
// own block hashes remain the tamper-evidence of record; the per-user chain is
// what MerkleRoot/VerifyIntegrity expose to the API. Per-user (rather than one
// global head) avoids an MVCC hot key on every write.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

const (
	eventKeyPrefix = "evt"
	headKeyPrefix  = "hd"
	eventName      = "finix.event"

	// defaultCoolingOffWindow mirrors spec §6.3 (and the retired in-memory rule).
	// It is the fallback used when no on-chain override is configured; see
	// resolveCoolingOffWindow / SetCoolingOffWindow (config~coolingOffWindow).
	defaultCoolingOffWindow = 6 * time.Hour

	// coolingOffConfigKey is the ledger state key holding an optional override of
	// the cooling-off duration, stored as a Go duration string such as "6h".
	coolingOffConfigKey = "config~coolingOffWindow"
)

// ValidCategories are the ledger categories. They mirror the `resource` argument
// used by the backend's WriteEvent call sites.
var ValidCategories = map[string]bool{
	"transaction":       true,
	"payment":           true,
	"consent":           true,
	"kyc":               true,
	"goal":              true,
	"security":          true,
	"audit":             true,
	"blockchain_record": true,
}

// Event mirrors backend blockchain.Event field-for-field so the gateway client
// can unmarshal it directly without a translation layer.
//
// NOTE: contractapi derives a JSON schema from this struct and validates every
// return value against it. Two consequences drive the field types below:
//   - Payload is a string, not json.RawMessage: []byte marshals to a JSON array
//     in the generated schema, which would reject the object we actually return.
//   - No `omitempty` anywhere: an omitted field fails the schema's "required".
type Event struct {
	ID            string              `json:"id"`
	UserID        string              `json:"userId"`
	Action        string              `json:"action"`
	Resource      string              `json:"resource"`
	Category      string              `json:"category"`
	PayloadHash   string              `json:"payloadHash"`
	PrevHash      string              `json:"prevHash"`
	Hash          string              `json:"hash"`
	Timestamp     time.Time           `json:"timestamp"`
	Seq           uint64              `json:"seq"`
	TxID          string              `json:"txId"`
	Payload       string              `json:"payload"`
	SmartContract SmartContractResult `json:"smartContract"`
}

// SmartContractResult records which rule ran and whether it passed.
type SmartContractResult struct {
	RuleName   string    `json:"ruleName"`
	Passed     bool      `json:"passed"`
	Violation  string    `json:"violation"`
	EnforcedAt time.Time `json:"enforcedAt"`
}

// chainHead tracks the per-user hash chain tip and sequence.
type chainHead struct {
	Seq  uint64 `json:"seq"`
	Hash string `json:"hash"`
}

// Proof is the chaincode-side integrity summary. Block-level proofs come from
// the peer's qscc (GetBlockByTxID), which chaincode cannot read.
// No `omitempty`: contractapi's generated schema marks every field required,
// so an omitted field fails return validation.
type Proof struct {
	UserID     string `json:"userId"`
	MerkleRoot string `json:"merkleRoot"`
	EventCount int    `json:"eventCount"`
	HeadHash   string `json:"headHash"`
	Intact     bool   `json:"intact"`
	Detail     string `json:"detail"`
}

// FinixContract is the chaincode entry point.
type FinixContract struct {
	contractapi.Contract
}

// ── Writes ──────────────────────────────────────────────────

// RecordEvent appends an event to the ledger under `category` for `userID`.
// payloadJSON is the raw event body (already JSON) supplied by the backend.
//
// Smart-contract rules are ENFORCED here: a violation returns an error, which
// aborts the transaction so nothing is committed. This is the guarantee the
// application layer cannot bypass.
func (c *FinixContract) RecordEvent(
	ctx contractapi.TransactionContextInterface,
	category string,
	userID string,
	action string,
	payloadJSON string,
) (*Event, error) {
	category = strings.TrimSpace(category)
	userID = strings.TrimSpace(userID)
	action = strings.TrimSpace(action)

	if userID == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}
	if !ValidCategories[category] {
		return nil, fmt.Errorf("unknown category %q: must be one of %s", category, categoryList())
	}
	if payloadJSON == "" {
		payloadJSON = "{}"
	}
	if !json.Valid([]byte(payloadJSON)) {
		return nil, fmt.Errorf("payload must be valid JSON")
	}

	stub := ctx.GetStub()

	// Deterministic clock: the transaction timestamp is identical on every peer.
	ts, err := stub.GetTxTimestamp()
	if err != nil {
		return nil, fmt.Errorf("get tx timestamp: %w", err)
	}
	now := time.Unix(ts.Seconds, int64(ts.Nanos)).UTC()

	head, err := readHead(ctx, userID)
	if err != nil {
		return nil, err
	}

	payloadHash := sha256Hex([]byte(payloadJSON))
	seq := head.Seq + 1

	ev := Event{
		ID:          fmt.Sprintf("%s-%d", userID, seq),
		UserID:      userID,
		Action:      action,
		Resource:    category,
		Category:    category,
		PayloadHash: payloadHash,
		PrevHash:    head.Hash,
		Timestamp:   now,
		Seq:         seq,
		TxID:        stub.GetTxID(),
		Payload:     payloadJSON,
	}
	ev.Hash = eventHash(ev)

	// Enforce the smart-contract rules against this user's existing history.
	history, err := queryByPartial(ctx, []string{userID})
	if err != nil {
		return nil, err
	}
	result, err := enforceRules(ev, history, now, resolveCoolingOffWindow(ctx))
	if err != nil {
		return nil, err // rule violation -> transaction rejected, nothing written
	}
	ev.SmartContract = result

	if err := putEvent(ctx, ev); err != nil {
		return nil, err
	}
	if err := writeHead(ctx, userID, chainHead{Seq: seq, Hash: ev.Hash}); err != nil {
		return nil, err
	}

	// Emit for the backend's event listener -> Postgres off-chain sync.
	evBytes, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}
	if err := stub.SetEvent(eventName, evBytes); err != nil {
		return nil, fmt.Errorf("set event: %w", err)
	}

	return &ev, nil
}

// ── Reads ───────────────────────────────────────────────────

// QueryByUser returns every event for a user, oldest first.
func (c *FinixContract) QueryByUser(
	ctx contractapi.TransactionContextInterface, userID string,
) ([]*Event, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("userID is required")
	}
	return queryByPartial(ctx, []string{userID})
}

// QueryByCategory returns a user's events in one category, oldest first.
func (c *FinixContract) QueryByCategory(
	ctx contractapi.TransactionContextInterface, userID string, category string,
) ([]*Event, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("userID is required")
	}
	if !ValidCategories[category] {
		return nil, fmt.Errorf("unknown category %q: must be one of %s", category, categoryList())
	}
	return queryByPartial(ctx, []string{userID, category})
}

// GetEvent returns a single event by user + sequence.
func (c *FinixContract) GetEvent(
	ctx contractapi.TransactionContextInterface, userID, category string, seq uint64,
) (*Event, error) {
	key, err := eventKey(ctx, userID, category, seq)
	if err != nil {
		return nil, err
	}
	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("event not found: %s/%s/%d", userID, category, seq)
	}
	var ev Event
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}
	return &ev, nil
}

// GetProof recomputes the user's hash chain and Merkle root on-chain, so the
// caller can verify the ledger has not been tampered with. Mirrors the retired
// VerifyIntegrity + MerkleRoot behaviour.
func (c *FinixContract) GetProof(
	ctx contractapi.TransactionContextInterface, userID string,
) (*Proof, error) {
	events, err := queryByPartial(ctx, []string{userID})
	if err != nil {
		return nil, err
	}
	p := &Proof{UserID: userID, EventCount: len(events), Intact: true}
	if len(events) == 0 {
		return p, nil
	}

	prev := ""
	hashes := make([]string, 0, len(events))
	for _, ev := range events {
		if ev.PrevHash != prev {
			p.Intact = false
			p.Detail = fmt.Sprintf("hash chain broken at %s: expected prev %q, got %q", ev.ID, prev, ev.PrevHash)
			break
		}
		if got := eventHash(*ev); got != ev.Hash {
			p.Intact = false
			p.Detail = fmt.Sprintf("hash mismatch at %s", ev.ID)
			break
		}
		prev = ev.Hash
		hashes = append(hashes, ev.Hash)
	}
	p.HeadHash = prev
	p.MerkleRoot = merkleRoot(hashes)
	return p, nil
}

// VerifyCoolingOff reports whether the user is inside a cooling-off window.
// Enforced on-chain so an application-layer bypass cannot release funds early.
func (c *FinixContract) VerifyCoolingOff(
	ctx contractapi.TransactionContextInterface, userID string,
) (map[string]interface{}, error) {
	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return nil, fmt.Errorf("get tx timestamp: %w", err)
	}
	now := time.Unix(ts.Seconds, int64(ts.Nanos)).UTC()

	events, err := queryByPartial(ctx, []string{userID, "transaction"})
	if err != nil {
		return nil, err
	}
	active, remaining := coolingOffState(events, now, resolveCoolingOffWindow(ctx))
	return map[string]interface{}{
		"active":           active,
		"remainingSeconds": int64(remaining.Seconds()),
	}, nil
}

// SetCoolingOffWindow overrides the cooling-off duration on-chain via the
// config~coolingOffWindow state key. duration is a Go duration string, e.g.
// "6h" or "90m"; it must be positive. Reverting to the default is done by
// setting it back to "6h".
func (c *FinixContract) SetCoolingOffWindow(ctx contractapi.TransactionContextInterface, duration string) error {
	d, err := time.ParseDuration(strings.TrimSpace(duration))
	if err != nil || d <= 0 {
		return fmt.Errorf("invalid cooling-off duration %q: expected a positive Go duration such as \"6h\"", duration)
	}
	return ctx.GetStub().PutState(coolingOffConfigKey, []byte(d.String()))
}

// GetCoolingOffWindow returns the effective cooling-off duration string (the
// on-chain override if set, otherwise the 6h default).
func (c *FinixContract) GetCoolingOffWindow(ctx contractapi.TransactionContextInterface) (string, error) {
	return resolveCoolingOffWindow(ctx).String(), nil
}

// resolveCoolingOffWindow returns the on-chain override from config~coolingOffWindow
// when present and valid, otherwise defaultCoolingOffWindow (6h).
func resolveCoolingOffWindow(ctx contractapi.TransactionContextInterface) time.Duration {
	data, err := ctx.GetStub().GetState(coolingOffConfigKey)
	if err != nil || len(data) == 0 {
		return defaultCoolingOffWindow
	}
	d, perr := time.ParseDuration(string(data))
	if perr != nil || d <= 0 {
		return defaultCoolingOffWindow
	}
	return d
}

// ── Smart contract rules (ported from the in-memory ledger) ──

// enforceRules runs every rule. Any violation aborts the transaction, so the
// rule is genuinely enforced rather than merely recorded.
func enforceRules(ev Event, history []*Event, now time.Time, coolingWindow time.Duration) (SmartContractResult, error) {
	rules := []func(Event, []*Event, time.Time) SmartContractResult{
		func(ev Event, h []*Event, now time.Time) SmartContractResult {
			return coolingOffRule(ev, h, now, coolingWindow)
		},
		consentRevocationRule,
		doubleSpendRule,
	}
	passed := SmartContractResult{RuleName: "all", Passed: true, EnforcedAt: now}
	for _, rule := range rules {
		res := rule(ev, history, now)
		if !res.Passed {
			return res, fmt.Errorf("smart contract %q rejected transaction: %s", res.RuleName, res.Violation)
		}
		passed.RuleName = res.RuleName
	}
	passed.RuleName = "all"
	return passed, nil
}

// coolingOffRule blocks a new transaction while a prior blocked transaction is
// still inside the cooling-off window (spec §6.3). The window is resolved by the
// caller (on-chain override or the 6h default) and passed in.
func coolingOffRule(ev Event, history []*Event, now time.Time, window time.Duration) SmartContractResult {
	res := SmartContractResult{RuleName: "cooling_off_enforcement", Passed: true, EnforcedAt: now}
	if ev.Action != "transaction_initiated" && ev.Action != "transaction_override" {
		return res
	}
	for _, prev := range history {
		if prev.UserID != ev.UserID || prev.Action != "transaction_blocked" {
			continue
		}
		if ev.Timestamp.Sub(prev.Timestamp) < window {
			res.Passed = false
			res.Violation = fmt.Sprintf("cooling-off period active: a blocked transaction occurred within the last %s", window)
			return res
		}
	}
	return res
}

// consentRevocationRule forbids data access after consent was revoked.
func consentRevocationRule(ev Event, history []*Event, now time.Time) SmartContractResult {
	res := SmartContractResult{RuleName: "consent_revocation_enforcement", Passed: true, EnforcedAt: now}
	if ev.Resource != "account_aggregator" && ev.Resource != "data_export" &&
		ev.Action != "account_aggregator_sync" && ev.Action != "data_exported" {
		return res
	}
	revoked := false
	for _, prev := range history {
		if prev.UserID != ev.UserID {
			continue
		}
		switch prev.Action {
		case "consent_revoked":
			if prev.Timestamp.Before(ev.Timestamp) {
				revoked = true
			}
		case "consent_granted", "consent_updated":
			if prev.Timestamp.Before(ev.Timestamp) {
				revoked = false // a later grant re-enables access
			}
		}
	}
	if revoked {
		res.Passed = false
		res.Violation = "consent was revoked before this data access event"
	}
	return res
}

// doubleSpendRule rejects a replay of an identical transaction payload.
func doubleSpendRule(ev Event, history []*Event, now time.Time) SmartContractResult {
	res := SmartContractResult{RuleName: "double_spend_prevention", Passed: true, EnforcedAt: now}
	if ev.Action != "transaction_initiated" {
		return res
	}
	for _, prev := range history {
		if prev.UserID != ev.UserID || prev.Action != "transaction_initiated" {
			continue
		}
		if prev.PayloadHash == ev.PayloadHash && !prev.Timestamp.Equal(ev.Timestamp) {
			res.Passed = false
			res.Violation = "duplicate transaction detected: identical payload already recorded"
			return res
		}
	}
	return res
}

// coolingOffState reports the remaining cooling-off window, if any, for the
// given window duration (resolved by the caller from the on-chain override or
// the 6h default).
func coolingOffState(events []*Event, now time.Time, window time.Duration) (bool, time.Duration) {
	var latest time.Time
	for _, ev := range events {
		if ev.Action != "transaction_blocked" {
			continue
		}
		if ev.Timestamp.After(latest) {
			latest = ev.Timestamp
		}
	}
	if latest.IsZero() {
		return false, 0
	}
	end := latest.Add(window)
	if now.Before(end) {
		return true, end.Sub(now)
	}
	return false, 0
}

// ── State helpers ───────────────────────────────────────────

func eventKey(ctx contractapi.TransactionContextInterface, userID, category string, seq uint64) (string, error) {
	// Zero-pad the sequence so the composite-key range scan returns events in
	// chronological order (lexicographic == numeric).
	return ctx.GetStub().CreateCompositeKey(eventKeyPrefix, []string{
		userID, category, fmt.Sprintf("%020d", seq),
	})
}

func putEvent(ctx contractapi.TransactionContextInterface, ev Event) error {
	key, err := eventKey(ctx, ev.UserID, ev.Category, ev.Seq)
	if err != nil {
		return fmt.Errorf("create composite key: %w", err)
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if err := ctx.GetStub().PutState(key, data); err != nil {
		return fmt.Errorf("put state: %w", err)
	}
	return nil
}

func headKey(userID string) string { return headKeyPrefix + "~" + userID }

func readHead(ctx contractapi.TransactionContextInterface, userID string) (chainHead, error) {
	data, err := ctx.GetStub().GetState(headKey(userID))
	if err != nil {
		return chainHead{}, fmt.Errorf("get head: %w", err)
	}
	if data == nil {
		return chainHead{Seq: 0, Hash: ""}, nil
	}
	var h chainHead
	if err := json.Unmarshal(data, &h); err != nil {
		return chainHead{}, fmt.Errorf("unmarshal head: %w", err)
	}
	return h, nil
}

func writeHead(ctx contractapi.TransactionContextInterface, userID string, h chainHead) error {
	data, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("marshal head: %w", err)
	}
	if err := ctx.GetStub().PutState(headKey(userID), data); err != nil {
		return fmt.Errorf("put head: %w", err)
	}
	return nil
}

// queryByPartial range-scans the composite key and returns events ordered by seq.
func queryByPartial(ctx contractapi.TransactionContextInterface, attrs []string) ([]*Event, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(eventKeyPrefix, attrs)
	if err != nil {
		return nil, fmt.Errorf("partial composite key query: %w", err)
	}
	defer func() { _ = iter.Close() }()

	out := make([]*Event, 0, 16)
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, fmt.Errorf("iterate: %w", err)
		}
		var ev Event
		if err := json.Unmarshal(kv.Value, &ev); err != nil {
			return nil, fmt.Errorf("unmarshal event: %w", err)
		}
		out = append(out, &ev)
	}
	// A scan across categories interleaves them; sort to a single chronological
	// order (seq is globally monotonic per user).
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// ── Hashing ─────────────────────────────────────────────────

// eventHash binds every immutable field of the event to its predecessor.
func eventHash(ev Event) string {
	return sha256Hex([]byte(strings.Join([]string{
		ev.PrevHash,
		ev.PayloadHash,
		ev.UserID,
		ev.Action,
		ev.Resource,
		strconv.FormatUint(ev.Seq, 10),
		ev.Timestamp.UTC().Format(time.RFC3339Nano),
	}, "|")))
}

func merkleRoot(hashes []string) string {
	if len(hashes) == 0 {
		return ""
	}
	level := make([]string, len(hashes))
	copy(level, hashes)
	for len(level) > 1 {
		next := make([]string, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 >= len(level) {
				next = append(next, sha256Hex([]byte(level[i]+level[i])))
				continue
			}
			next = append(next, sha256Hex([]byte(level[i]+level[i+1])))
		}
		level = next
	}
	return level[0]
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func categoryList() string {
	out := make([]string, 0, len(ValidCategories))
	for c := range ValidCategories {
		out = append(out, c)
	}
	sort.Strings(out) // deterministic error message
	return strings.Join(out, ", ")
}

func main() {
	cc, err := contractapi.NewChaincode(&FinixContract{})
	if err != nil {
		panic(fmt.Sprintf("create FINIX chaincode: %v", err))
	}
	if err := cc.Start(); err != nil {
		panic(fmt.Sprintf("start FINIX chaincode: %v", err))
	}
}
