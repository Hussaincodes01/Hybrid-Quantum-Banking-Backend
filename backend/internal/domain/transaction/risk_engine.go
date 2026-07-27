package transaction

import (
	"log/slog"
	"os"
	"strings"

	"FINIX/backend/internal/config"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// RiskSignal is Model 1's feature vector (FINIX_Srishti §2.1). The first nine
// fields are the original signals; the last four bring the struct to parity with
// the spec's 12-feature RiskSignal_vector and the master relational dataset
// (api-docs/"FINIX Dataset Features"). HourOfDay is retained as an implementation
// extra (the dataset carries hour_of_day; the unusual-hour term supplements
// DayOfWeekZScore). The four new fields default to spec-valid neutral values until
// the upstream data plumbing (geo history, market-volatility feed, 24h recipient
// counts) is wired to populate them — see docs/SCHEMA_RECONCILIATION.md.
type RiskSignal struct {
	AmountVsAverage   float64 `json:"amountVsAverage"`
	IsNewRecipient    bool    `json:"isNewRecipient"`
	HourOfDay         int     `json:"hourOfDay"`
	FailedPINAttempts int     `json:"failedPinAttempts"`
	VelocityCount1Hr  int     `json:"velocityCount1Hr"`
	BalanceImpact     float64 `json:"balanceImpact"`
	RecipientGNNScore float64 `json:"recipientGnnScore"`
	BehaviourDrift    float64 `json:"behaviourDrift"`
	SessionTrustScore float64 `json:"sessionTrustScore"`

	// Spec §2.1.1 terms — added for schema parity with the canonical dataset.
	GeoVelocityFlag   bool    `json:"geoVelocityFlag"`   // impossible travel: Haversine/hours > 900 km/h
	DayOfWeekZScore   float64 `json:"dayOfWeekZScore"`   // 0 when < 4 same-weekday observations (excluded, §2.1.1)
	MarketContextTerm bool    `json:"marketContextTerm"` // realised vol > 2× 30-day avg
	StructuringFlag   bool    `json:"structuringFlag"`   // smurfing pattern (3+ uniform sub-benchmark txns/24h)
}

type Assessment struct {
	Level        RiskLevel `json:"level"`
	Score        float64   `json:"score"`
	Reason       string    `json:"reason"`
	ModelVersion string    `json:"model_version"`
}

type RiskEngine struct {
	predictor ModelPredictor
	heuristic *HeuristicPredictor
	amt       config.AmountTermParams
}

func NewRiskEngine() RiskEngine {
	return NewRiskEngineWithParams(config.Default().AmountTerm)
}

// NewRiskEngineWithParams builds the engine with a specific (versioned) amount-term
// parameter set so thresholds/weights are auditable config, not magic literals.
func NewRiskEngineWithParams(amt config.AmountTermParams) RiskEngine {
	modelPath := strings.TrimSpace(os.Getenv("FINIX_RISK_MODEL_PATH"))
	if modelPath == "" {
		modelPath = "../../models/security/fraud_label/v1/model.onnx"
	}

	heuristic := &HeuristicPredictor{amt: amt}
	predictor := NewONNXPredictor(modelPath)

	return RiskEngine{predictor: predictor, heuristic: heuristic, amt: amt}
}

func (e RiskEngine) Evaluate(sig RiskSignal) Assessment {
	// Clip ratios / resolve NaN/Inf before ANY model path (ONNX or heuristic) so
	// a degenerate ratio can never reach the weighting step (spec §1.1).
	sig = normaliseSignal(sig, e.amt)

	if e.predictor.IsAvailable() {
		features := signalToFeatures(sig)
		probs, err := e.predictor.Predict(features)
		if err == nil && len(probs) == 3 {
			slog.Debug("onnx model inference succeeded")
			return assessmentFromProbs(probs, sig)
		}
		slog.Warn("onnx inference failed, falling back to heuristic", "error", err)
	}

	return e.heuristic.assess(sig)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
