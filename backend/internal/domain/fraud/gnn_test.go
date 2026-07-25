package fraud

import (
	"fmt"
	"math"
	"testing"
)

// almostEqual compares floats within a small epsilon.
func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestReverseEdgePropagation verifies that risk from a fraud sender propagates to
// a downstream node via the reverse index (fraud -> A), matching the decay model.
func TestReverseEdgePropagation(t *testing.T) {
	fg := NewFraudGraph()
	// A single txn from a known-fraud recipient into "clean-a".
	fg.MarkAsFraud("bad-actor")
	fg.AddTransaction("bad-actor", "clean-a", 100000)

	got := fg.GetRiskScore("clean-a")
	// One txn => edge weight 1/10 = 0.1; propagated = 1.0 * decay(0.6) * 0.1 = 0.06.
	want := fraudNodeRisk * riskDecayFactor * 0.1
	if !almostEqual(got, want) {
		t.Fatalf("reverse propagation: got %v, want %v", got, want)
	}
}

// TestKnownNodeIgnoresPatternKeyword ensures a known low-risk node whose name
// contains a scam keyword is scored by graph structure, NOT the pattern override.
func TestKnownNodeIgnoresPatternKeyword(t *testing.T) {
	fg := NewFraudGraph()
	// "new-offer-store" contains medium-risk tokens but has only clean activity.
	fg.AddTransaction("payer", "new-offer-store", 50000)

	got := fg.GetRiskScore("new-offer-store")
	if got > 0.2 {
		t.Fatalf("known clean node scored by keyword override: got %v, want low graph risk", got)
	}
}

// TestMemoInvalidation checks that a cached score is recomputed after a mutation.
func TestMemoInvalidation(t *testing.T) {
	fg := NewFraudGraph()
	fg.AddTransaction("payer", "target", 50000)

	first := fg.GetRiskScore("target") // caches a low score
	if first > 0.2 {
		t.Fatalf("expected low initial risk, got %v", first)
	}

	// Mark an upstream sender as fraud; the cached score must be invalidated.
	fg.MarkAsFraud("payer")
	second := fg.GetRiskScore("target")
	if !(second > first) {
		t.Fatalf("memo not invalidated after mutation: first=%v second=%v", first, second)
	}
}

// TestHashWidth guards the 128-bit node id (32 hex chars) and distinctness.
func TestHashWidth(t *testing.T) {
	a := hashRecipient("alice")
	b := hashRecipient("bob")
	if a == b {
		t.Fatal("distinct recipients produced identical node ids")
	}
	if len(a) != 32 {
		t.Fatalf("expected 32 hex chars (128-bit), got %d", len(a))
	}
	if hashRecipient("Alice ") != hashRecipient("alice") {
		t.Fatal("hash not normalising case/whitespace")
	}
}

func TestPatternScoring(t *testing.T) {
	cases := []struct {
		in      string
		maxWant float64 // score must be <= this
		minWant float64 // and >= this
	}{
		{"investment opportunity", 0.3, 0.0}, // "investment" != token "invest"; no false positive
		{"invest now", 0.5, 0.5},             // standalone "invest" token matches medium risk
		{"renew subscription", 0.2, 0.0},     // "renew" must NOT match "new" (substring bug)
		{"unknown urgent lottery", 1.0, 0.9}, // high-risk tokens preserved
		{"HDFC Bank", 0.2, 0.0},              // legit name, low prior
	}
	for _, c := range cases {
		got := patternBasedScore(c.in)
		if got < c.minWant || got > c.maxWant {
			t.Errorf("patternBasedScore(%q) = %v, want in [%v, %v]", c.in, got, c.minWant, c.maxWant)
		}
	}
}

// TestPatternInvestmentNotHighRisk pins the specific false-positive the audit
// flagged: "investment" (and substrings like "invest" inside a word) must not
// return the 0.9 high-risk score.
func TestPatternInvestmentNotHighRisk(t *testing.T) {
	if got := patternBasedScore("investment opportunity"); got >= 0.9 {
		t.Fatalf("investment mis-scored as high risk: %v", got)
	}
}

func BenchmarkGetRiskScore10kGraph(b *testing.B) {
	fg := NewFraudGraph()
	// Seed a 10k-node graph with a modest fan-out so reverse lookups are exercised.
	const n = 10000
	for i := 0; i < n; i++ {
		from := fmt.Sprintf("acct-%d", i)
		to := fmt.Sprintf("acct-%d", (i*7+1)%n)
		fg.AddTransaction(from, to, int64(1000+i))
	}
	fg.MarkAsFraud("acct-3")
	target := "acct-1234"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fg.GetRiskScore(target)
	}
}
