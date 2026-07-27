package transaction

import (
	"fmt"
	"strings"

	"FINIX/backend/internal/config"
)

type HeuristicPredictor struct {
	amt config.AmountTermParams
}

// featureCount is the length of the Model 1 feature vector, kept identical between
// signalToFeatures (Go) and feature_cols in ml/train_fraud_model.py. Bump both in
// lockstep — the ONNX model's input width is derived from this.
const featureCount = 13

func (h *HeuristicPredictor) Predict(features []float32) ([]float32, error) {
	if len(features) < featureCount {
		return nil, fmt.Errorf("heuristic predictor requires %d features, got %d", featureCount, len(features))
	}

	sig := RiskSignal{
		AmountVsAverage:   float64(features[0]),
		IsNewRecipient:    features[1] > 0.5,
		HourOfDay:         int(features[2]),
		FailedPINAttempts: int(features[3]),
		VelocityCount1Hr:  int(features[4]),
		BalanceImpact:     float64(features[5]),
		RecipientGNNScore: float64(features[6]),
		BehaviourDrift:    float64(features[7]),
		SessionTrustScore: float64(features[8]),
		GeoVelocityFlag:   features[9] > 0.5,
		DayOfWeekZScore:   float64(features[10]),
		MarketContextTerm: features[11] > 0.5,
		StructuringFlag:   features[12] > 0.5,
	}

	assessment := h.assess(sig)

	low, medium, high := 0.0, 0.0, 0.0
	switch assessment.Level {
	case RiskLow:
		low = 1.0
	case RiskMedium:
		medium = 1.0
	case RiskHigh:
		high = 1.0
	}

	return []float32{float32(low), float32(medium), float32(high)}, nil
}

func (h *HeuristicPredictor) IsAvailable() bool { return true }

func (h *HeuristicPredictor) assess(sig RiskSignal) Assessment {
	// Clip ratios and resolve NaN/Inf before any weighting (spec §1.1).
	sig = normaliseSignal(sig, h.amt)

	score := 0.0
	reasons := make([]string, 0, 6)

	if sig.AmountVsAverage > 1 {
		// g(x) = clip((x-1)·k, 0, cap) — versioned amount-anomaly term.
		amountScore := clampFloat((sig.AmountVsAverage-1.0)*h.amt.K, 0, h.amt.Cap, h.amt.Cap)
		score += amountScore
		if sig.AmountVsAverage > 3 {
			reasons = append(reasons, fmt.Sprintf("amount is %.1fx your usual transfer", sig.AmountVsAverage))
		}
	}

	if sig.IsNewRecipient {
		score += 15
		reasons = append(reasons, "recipient is new or unverified")
	}

	if sig.HourOfDay <= 4 || sig.HourOfDay >= 23 {
		score += 10
		reasons = append(reasons, "transaction requested at an unusual hour")
	}

	if sig.FailedPINAttempts > 0 {
		pinScore := minFloat(float64(sig.FailedPINAttempts*5), 15)
		score += pinScore
		reasons = append(reasons, "multiple recent PIN failures")
	}

	if sig.VelocityCount1Hr >= 3 {
		velocityScore := minFloat(float64(sig.VelocityCount1Hr*2), 10)
		score += velocityScore
		reasons = append(reasons, "high transaction velocity in the last hour")
	}

	if sig.BalanceImpact > 0.8 {
		score += 10
		reasons = append(reasons, "this transfer would materially drain your balance")
	} else if sig.BalanceImpact > 0.5 {
		score += 5
	}

	if sig.RecipientGNNScore > 0 {
		score += minFloat(sig.RecipientGNNScore*20, 20)
		if sig.RecipientGNNScore >= 0.6 {
			reasons = append(reasons, "recipient appears close to known high-risk payment graph nodes")
		}
	}

	if sig.BehaviourDrift > 0 {
		score += minFloat(sig.BehaviourDrift*20, 20)
		if sig.BehaviourDrift >= 0.6 {
			reasons = append(reasons, "session behaviour differs from your normal interaction pattern")
		}
	}

	if sig.SessionTrustScore < 0.4 {
		score += 15
		reasons = append(reasons, "device and session trust score is low")
	} else if sig.SessionTrustScore < 0.7 {
		score += 8
	}

	// Spec §2.1.3 rule-based terms (w6/w7/w8/w12). These stay inert until the
	// upstream data plumbing populates the fields (see SCHEMA_RECONCILIATION.md),
	// so behaviour is unchanged for callers that don't yet set them.
	if sig.GeoVelocityFlag {
		score += 20
		reasons = append(reasons, "location change implies physically impossible travel")
	}
	if sig.DayOfWeekZScore > 0 {
		// Cap the day-of-week contribution at 10 points (§2.1.3 w7*10).
		score += minFloat(10*sig.DayOfWeekZScore, 10)
		if sig.DayOfWeekZScore >= 2 {
			reasons = append(reasons, "spend is unusual for this day of week")
		}
	}
	if sig.MarketContextTerm {
		score += 8
		reasons = append(reasons, "market is unusually volatile right now")
	}
	if sig.StructuringFlag {
		score += 25
		reasons = append(reasons, "several similarly-sized transfers in a short window (possible structuring)")
	}

	level := RiskLow
	switch {
	case score >= 70:
		level = RiskHigh
	case score >= 40:
		level = RiskMedium
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "transaction aligns with your normal behavior")
	}

	return Assessment{
		Level:  level,
		Score:  minFloat(score, 100),
		Reason: strings.Join(reasons, "; "),
	}
}

// signalToFeatures emits the feature vector in the EXACT order used by
// feature_cols in ml/train_fraud_model.py. The two MUST stay identical, or the
// ONNX model receives features in the wrong positions.
func signalToFeatures(sig RiskSignal) []float32 {
	return []float32{
		float32(sig.AmountVsAverage),       // 0
		boolToFloat(sig.IsNewRecipient),    // 1
		float32(sig.HourOfDay),             // 2
		float32(sig.FailedPINAttempts),     // 3
		float32(sig.VelocityCount1Hr),      // 4
		float32(sig.BalanceImpact),         // 5
		float32(sig.RecipientGNNScore),     // 6
		float32(sig.BehaviourDrift),        // 7
		float32(sig.SessionTrustScore),     // 8
		boolToFloat(sig.GeoVelocityFlag),   // 9
		float32(sig.DayOfWeekZScore),       // 10
		boolToFloat(sig.MarketContextTerm), // 11
		boolToFloat(sig.StructuringFlag),   // 12
	}
}

func assessmentFromProbs(probs []float32, sig RiskSignal) Assessment {
	low, medium, high := float64(probs[0]), float64(probs[1]), float64(probs[2])

	score := medium*50 + high*90 + low*10

	var level RiskLevel
	switch {
	case high >= 0.7:
		level = RiskHigh
	case high >= 0.4 || medium >= 0.6:
		level = RiskMedium
	default:
		level = RiskLow
	}

	reasons := make([]string, 0, 2)
	if level != RiskLow {
		reasons = append(reasons, "ML model flagged elevated risk")
	} else {
		reasons = append(reasons, "transaction aligns with your normal behavior")
	}

	return Assessment{
		Level:  level,
		Score:  minFloat(score, 100),
		Reason: strings.Join(reasons, "; "),
	}
}

func boolToFloat(b bool) float32 {
	if b {
		return 1.0
	}
	return 0.0
}
