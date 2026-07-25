package transaction

import (
	"math"

	"FINIX/backend/internal/config"
)

// clampFloat bounds x to [lo,hi]. A NaN or ±Inf input is replaced by nanInf — the
// caller chooses whether an undefined ratio should read as maximally-anomalous or
// as a safe default (spec §1.1: unclipped ratios must never reach the weighting
// step as NaN/Inf, which silently produce false-HIGH or false-LOW scores).
func clampFloat(x, lo, hi, nanInf float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return nanInf
	}
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// normaliseSignal clips every ratio-like field to a bounded range BEFORE it is
// weighted, and resolves NaN/Inf explicitly (spec §1.1). Risk-increasing ratios
// (amount, balance impact, GNN, drift) resolve an undefined value to their
// anomalous ceiling; the trust score — where LOW is the risky direction —
// resolves to its floor. The amount ratio is additionally capped at ClipMax so a
// 1000x outlier can't dominate the score.
func normaliseSignal(sig RiskSignal, amt config.AmountTermParams) RiskSignal {
	sig.AmountVsAverage = clampFloat(sig.AmountVsAverage, 0, amt.ClipMax, amt.ClipMax)
	sig.BalanceImpact = clampFloat(sig.BalanceImpact, 0, amt.BalanceClipMax, amt.BalanceClipMax)
	sig.RecipientGNNScore = clampFloat(sig.RecipientGNNScore, 0, 1, 1)
	sig.BehaviourDrift = clampFloat(sig.BehaviourDrift, 0, 1, 1)
	sig.SessionTrustScore = clampFloat(sig.SessionTrustScore, 0, 1, 0)
	return sig
}
