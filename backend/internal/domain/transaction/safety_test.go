package transaction

import (
	"math"
	"testing"

	"FINIX/backend/internal/config"
)

func newHeuristic() *HeuristicPredictor {
	return &HeuristicPredictor{amt: config.Default().AmountTerm}
}

func TestZeroHistoryNoNaNOrInf(t *testing.T) {
	h := newHeuristic()
	// A degenerate ratio (e.g. divide-by-zero upstream) must not crash the score.
	sig := RiskSignal{
		AmountVsAverage:   math.Inf(1),
		BalanceImpact:     math.NaN(),
		RecipientGNNScore: math.Inf(1),
		SessionTrustScore: math.NaN(),
	}
	a := h.assess(sig)
	if math.IsNaN(a.Score) || math.IsInf(a.Score, 0) {
		t.Fatalf("score must be finite, got %v", a.Score)
	}
	if a.Score < 0 || a.Score > 100 {
		t.Fatalf("score must be bounded [0,100], got %v", a.Score)
	}
}

func TestAmountRatioClipsToCeiling(t *testing.T) {
	h := newHeuristic()
	amt := config.Default().AmountTerm
	// A 1000x ratio must clip to the 25x ceiling before weighting, and the amount
	// term must not exceed its cap.
	big := RiskSignal{AmountVsAverage: 1000}
	small := RiskSignal{AmountVsAverage: amt.ClipMax} // exactly the ceiling
	if h.assess(big).Score != h.assess(small).Score {
		t.Fatalf("1000x should clip to %vx: scores differ (%v vs %v)",
			amt.ClipMax, h.assess(big).Score, h.assess(small).Score)
	}
	// The amount term alone is capped: (ClipMax-1)*K clipped to Cap.
	expectedAmountTerm := clampFloat((amt.ClipMax-1)*amt.K, 0, amt.Cap, amt.Cap)
	if expectedAmountTerm != amt.Cap {
		t.Fatalf("expected amount term to hit cap %v, got %v", amt.Cap, expectedAmountTerm)
	}
}

func TestBalanceImpactClipsToOne(t *testing.T) {
	sig := normaliseSignal(RiskSignal{BalanceImpact: 5.0}, config.Default().AmountTerm)
	if sig.BalanceImpact != 1.0 {
		t.Fatalf("balance impact must clip to 1.0, got %v", sig.BalanceImpact)
	}
	sig = normaliseSignal(RiskSignal{BalanceImpact: -3.0}, config.Default().AmountTerm)
	if sig.BalanceImpact != 0 {
		t.Fatalf("negative balance impact must clip to 0, got %v", sig.BalanceImpact)
	}
}

func TestNaNTrustResolvesToRisky(t *testing.T) {
	// SessionTrustScore is risky when LOW, so an undefined value must resolve to 0.
	sig := normaliseSignal(RiskSignal{SessionTrustScore: math.NaN()}, config.Default().AmountTerm)
	if sig.SessionTrustScore != 0 {
		t.Fatalf("NaN trust must resolve to 0 (risky), got %v", sig.SessionTrustScore)
	}
}

func TestNormaliseIsIdempotentOnCleanInput(t *testing.T) {
	amt := config.Default().AmountTerm
	clean := RiskSignal{AmountVsAverage: 2.0, BalanceImpact: 0.5, SessionTrustScore: 0.8, RecipientGNNScore: 0.3, BehaviourDrift: 0.1}
	once := normaliseSignal(clean, amt)
	twice := normaliseSignal(once, amt)
	if once != twice {
		t.Fatalf("normalise must be idempotent: %+v vs %+v", once, twice)
	}
	if once.AmountVsAverage != 2.0 {
		t.Fatalf("clean value must pass through unchanged, got %v", once.AmountVsAverage)
	}
}
