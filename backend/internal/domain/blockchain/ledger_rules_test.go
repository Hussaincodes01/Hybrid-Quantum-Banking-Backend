package blockchain

import (
	"context"
	"errors"
	"testing"
	"time"
)

// legacyEval is the pre-refactor linear-scan implementation of each rule, kept in
// the test only, so we can assert the indexed RuleQuery version is behaviourally
// identical on the same fixtures.
func legacyCoolingOff(event Event, events []Event) bool {
	if event.Action != "transaction_blocked" && event.Action != "transaction_initiated" {
		return true
	}
	for _, prev := range events {
		if prev.UserID != event.UserID || prev.Action != "transaction_blocked" {
			continue
		}
		if event.Timestamp.Sub(prev.Timestamp) < 6*time.Hour {
			return false
		}
	}
	return true
}

func legacyDoubleSpend(event Event, events []Event) bool {
	if event.Action != "transaction_initiated" {
		return true
	}
	for _, prev := range events {
		if prev.UserID != event.UserID {
			continue
		}
		if prev.Action == "transaction_initiated" && prev.PayloadHash == event.PayloadHash {
			if !prev.Timestamp.Equal(event.Timestamp) {
				return false
			}
		}
	}
	return true
}

func buildEvents() []Event {
	base := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	return []Event{
		{UserID: "u1", Action: "transaction_blocked", Timestamp: base.Add(-1 * time.Hour)},
		{UserID: "u1", Action: "transaction_initiated", PayloadHash: "hashA", Timestamp: base.Add(-3 * time.Hour)},
		{UserID: "u2", Action: "transaction_blocked", Timestamp: base.Add(-10 * time.Hour)},
		{UserID: "u2", Action: "consent_revoked", Timestamp: base.Add(-2 * time.Hour)},
	}
}

func TestCoolingOffEquivalence(t *testing.T) {
	events := buildEvents()
	idx := newEventIndex(events)
	rule := &CoolingOffRule{}
	now := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)

	cases := []Event{
		{UserID: "u1", Action: "transaction_initiated", Timestamp: now}, // within 6h of u1 block -> violation
		{UserID: "u2", Action: "transaction_initiated", Timestamp: now}, // u2 block was 10h ago -> pass
		{UserID: "u3", Action: "transaction_initiated", Timestamp: now}, // no history -> pass
	}
	for _, ev := range cases {
		gotPassed := rule.Evaluate(ev, idx).Passed
		wantPassed := legacyCoolingOff(ev, events)
		if gotPassed != wantPassed {
			t.Errorf("cooling-off %s: indexed=%v legacy=%v", ev.UserID, gotPassed, wantPassed)
		}
	}
}

func TestDoubleSpendEquivalence(t *testing.T) {
	events := buildEvents()
	idx := newEventIndex(events)
	rule := &DoubleSpendRule{}
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	cases := []Event{
		{UserID: "u1", Action: "transaction_initiated", PayloadHash: "hashA", Timestamp: now}, // dup -> violation
		{UserID: "u1", Action: "transaction_initiated", PayloadHash: "hashZ", Timestamp: now}, // new -> pass
		{UserID: "u3", Action: "transaction_initiated", PayloadHash: "hashA", Timestamp: now}, // diff user -> pass
	}
	for i, ev := range cases {
		gotPassed := rule.Evaluate(ev, idx).Passed
		wantPassed := legacyDoubleSpend(ev, events)
		if gotPassed != wantPassed {
			t.Errorf("double-spend case %d: indexed=%v legacy=%v", i, gotPassed, wantPassed)
		}
	}
}

func TestConsentRevocationIndexed(t *testing.T) {
	events := buildEvents()
	idx := newEventIndex(events)
	rule := &ConsentRevocationRule{}
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	// u2 revoked consent before this data-access event -> violation.
	ev := Event{UserID: "u2", Action: "data_access", Resource: "account_aggregator", Timestamp: now}
	if rule.Evaluate(ev, idx).Passed {
		t.Error("expected consent-revocation violation for u2 data access after revocation")
	}
	// u1 never revoked -> pass.
	ev2 := Event{UserID: "u1", Action: "data_access", Resource: "data_export", Timestamp: now}
	if !rule.Evaluate(ev2, idx).Passed {
		t.Error("expected pass for u1 with no revocation")
	}
}

// TestEndToEndWriteEvent exercises the full WriteEvent path (index built once,
// all three rules run) to confirm the wiring compiles and enforces as before.
func TestEndToEndWriteEvent(t *testing.T) {
	l := NewLedger()
	// First initiated transaction: should pass.
	ev1, _, err := l.writeEventInMemory("u9", "transaction_initiated", "transaction", map[string]any{"amt": 100})
	if err != nil {
		t.Fatal(err)
	}
	if !ev1.SmartContract.Passed {
		t.Fatalf("first txn should pass, got violation %q", ev1.SmartContract.Violation)
	}
	// Duplicate payload (same map) at a later time: double-spend violation.
	ev2, violations, err := l.writeEventInMemory("u9", "transaction_initiated", "transaction", map[string]any{"amt": 100})
	if err != nil {
		t.Fatal(err)
	}
	if ev2.SmartContract.Passed {
		t.Fatal("duplicate payload should trip double-spend rule")
	}
	if len(violations) == 0 {
		t.Fatal("expected the duplicate to report a violation list")
	}
}

// TestWriteEventStrictRejectsViolation confirms the H-8 enforcement: the strict
// write must return ErrSmartContractViolation on a double-spend, while the
// audit-only WriteEvent records the same event with a nil error.
func TestWriteEventStrictRejectsViolation(t *testing.T) {
	ctx := context.Background()

	// Audit-only path: duplicate is recorded, no error (records for audit).
	la := NewLedger()
	if _, err := la.WriteEvent(ctx, "u1", "transaction_initiated", "transaction", map[string]any{"amt": 100}); err != nil {
		t.Fatalf("first audit write: %v", err)
	}
	if _, err := la.WriteEvent(ctx, "u1", "transaction_initiated", "transaction", map[string]any{"amt": 100}); err != nil {
		t.Fatalf("audit-only WriteEvent must not error on violation, got %v", err)
	}

	// Strict path: first passes, duplicate is rejected.
	ls := NewLedger()
	if _, err := ls.WriteEventStrict(ctx, "u1", "transaction_initiated", "transaction", map[string]any{"amt": 100}); err != nil {
		t.Fatalf("first strict write should pass: %v", err)
	}
	_, err := ls.WriteEventStrict(ctx, "u1", "transaction_initiated", "transaction", map[string]any{"amt": 100})
	if !errors.Is(err, ErrSmartContractViolation) {
		t.Fatalf("strict duplicate must return ErrSmartContractViolation, got %v", err)
	}
}

func BenchmarkRuleEvalLargeLedger(b *testing.B) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := make([]Event, 0, 100000)
	for i := 0; i < 100000; i++ {
		events = append(events, Event{
			UserID:      "u" + string(rune('a'+i%26)),
			Action:      "transaction_initiated",
			PayloadHash: "h" + string(rune(i)),
			Timestamp:   base.Add(time.Duration(i) * time.Minute),
		})
	}
	rule := &DoubleSpendRule{}
	probe := Event{UserID: "ua", Action: "transaction_initiated", PayloadHash: "hx", Timestamp: base}

	idx := newEventIndex(events) // build once; benchmark measures per-rule lookup only
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rule.Evaluate(probe, idx)
	}
}
