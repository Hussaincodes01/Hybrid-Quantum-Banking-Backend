package credit

import (
	"testing"
	"time"

	"FINIX/backend/internal/config"
)

func TestUtilisationBoundary(t *testing.T) {
	// Exactly at the 0.30 threshold counts as breached (>=).
	if u := Utilisation(30000, 100000); u != 0.30 {
		t.Fatalf("utilisation = %v, want 0.30", u)
	}
	if u := Utilisation(100, 0); u != 0 {
		t.Fatalf("zero-limit utilisation = %v, want 0", u)
	}
}

func TestAggregateHidesMaxedCard(t *testing.T) {
	// Aggregate looks healthy (well under 0.30) but card B is maxed — per-card must
	// surface it.
	cards := []Card{
		{ID: "A", OutstandingPaise: 0, LimitPaise: 500_00000},
		{ID: "B", OutstandingPaise: 95_00000, LimitPaise: 100_00000},
	}
	agg := AggregateUtilisation(cards)
	if agg >= 0.30 {
		t.Fatalf("aggregate = %.3f, expected < 0.30 (masking effect)", agg)
	}
	now := time.Now()
	// Statement 3 days out for card B → within T-5 window.
	cards[1].StatementDate = now.Add(3 * 24 * time.Hour)
	rep := EvaluateUtilisation(cards, now, config.Default().Credit)
	if !rep.AnyAlert {
		t.Fatal("a maxed single card within lead time must raise an alert despite a healthy aggregate")
	}
}

func TestLeadTimeAlertFiresBeforeStatementNotAfter(t *testing.T) {
	p := config.Default().Credit
	now := time.Now()
	// Maxed card, statement 3 days ahead → alert.
	before := []Card{{ID: "X", OutstandingPaise: 50_00000, LimitPaise: 100_00000, StatementDate: now.Add(3 * 24 * time.Hour)}}
	if !EvaluateUtilisation(before, now, p).AnyAlert {
		t.Fatal("alert should fire within the T-5 window before the statement")
	}
	// Same card, statement already 2 days PAST → no lead-time alert.
	after := []Card{{ID: "X", OutstandingPaise: 50_00000, LimitPaise: 100_00000, StatementDate: now.Add(-2 * 24 * time.Hour)}}
	if EvaluateUtilisation(after, now, p).AnyAlert {
		t.Fatal("alert must not fire after the statement date has passed")
	}
	// Maxed card, statement 20 days ahead → outside lead window → no alert yet.
	far := []Card{{ID: "X", OutstandingPaise: 50_00000, LimitPaise: 100_00000, StatementDate: now.Add(20 * 24 * time.Hour)}}
	if EvaluateUtilisation(far, now, p).AnyAlert {
		t.Fatal("alert must not fire outside the T-5 lead window")
	}
}

func TestEffectiveAPRExposesHiddenCost(t *testing.T) {
	// "0% headline" BNPL: borrow ₹10,000, repay ₹10,300 over 30 days.
	// Effective APR = (0.03) * (365/30) ≈ 36.5%.
	apr := EffectiveAPR(1_030_000, 1_000_000, 30)
	if apr < 0.35 || apr > 0.38 {
		t.Fatalf("effective APR = %.4f, want ≈ 0.365 (36.5%%)", apr)
	}
	// Genuinely free → 0.
	if apr := EffectiveAPR(1_000_000, 1_000_000, 30); apr != 0 {
		t.Fatalf("free BNPL APR = %v, want 0", apr)
	}
}

func TestIsBNPLObligation(t *testing.T) {
	p := config.Default().Credit
	if !IsBNPLObligation("", "LazyPay India Pvt Ltd", p) {
		t.Fatal("LazyPay counterparty should classify as BNPL")
	}
	if !IsBNPLObligation("6012", "Some Random Merchant", p) {
		t.Fatal("BNPL MCC should classify as BNPL")
	}
	if IsBNPLObligation("5411", "Big Bazaar Grocery", p) {
		t.Fatal("a grocery merchant must not classify as BNPL")
	}
}

func TestTotalBNPLOutstanding(t *testing.T) {
	obs := []Obligation{
		{Provider: "LazyPay", DueAmountPaise: 320000},
		{Provider: "Simpl", DueAmountPaise: 150000},
		{Provider: "bad", DueAmountPaise: -50}, // ignored
	}
	if got := TotalBNPLOutstanding(obs); got != 470000 {
		t.Fatalf("total BNPL outstanding = %d, want 470000", got)
	}
}
