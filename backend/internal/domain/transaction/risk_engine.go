package transaction

import (
	"log/slog"
	"os"
	"strings"

	"FINIX/backend/internal/config"
	"FINIX/backend/internal/ml/preprocess"
)

// txnFeatureCount is the input width of transaction_risk_model.onnx (see
// MODEL_IO_CONTRACT.md and the training-notebook FEATURE_COLUMNS).
const txnFeatureCount = 32

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

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
	IsColdStart       bool    `json:"-"`

	// --- Extended raw inputs for the 32-feature transaction_risk_model.onnx ---
	// Mapped to the model's exact feature names by featureMaps (risk_features.go).
	// Amounts are in RUPEES (not paise) to match the training-data units. Any
	// field left at its zero value is treated as "not provided" and defaults to
	// the training mean at inference (scaled to 0); the boolean posture flags are
	// always sent (false == a real 0). These are only consumed when the ONNX
	// model + its preprocess artifact are both present; otherwise the heuristic
	// (which reads only the 9 core signals above) is used.
	TransactionAmount       float64 `json:"transactionAmount,omitempty"`
	TransactionsLast24h     int     `json:"transactionsLast24h,omitempty"`
	AccountAgeDays          int     `json:"accountAgeDays,omitempty"`
	CurrentBalance          float64 `json:"currentBalance,omitempty"`
	MonthlyIncome           float64 `json:"monthlyIncome,omitempty"`
	RegisteredDeviceMatch   bool    `json:"registeredDeviceMatch,omitempty"`
	RootedDevice            bool    `json:"rootedDevice,omitempty"`
	EmulatorDetected        bool    `json:"emulatorDetected,omitempty"`
	OTPVerified             bool    `json:"otpVerified,omitempty"`
	BiometricVerified       bool    `json:"biometricVerified,omitempty"`
	RecipientVerified       bool    `json:"recipientVerified,omitempty"`
	RecipientAccountAgeDays int     `json:"recipientAccountAgeDays,omitempty"`
	MarketVolatilityIndex   float64 `json:"marketVolatilityIndex,omitempty"`
	GeoVelocityFlag         bool    `json:"geoVelocityFlag,omitempty"`
	StructuringFlag         bool    `json:"structuringFlag,omitempty"`
	TransactionType         string  `json:"transactionType,omitempty"`
	PaymentChannel          string  `json:"paymentChannel,omitempty"`
	MerchantCategory        string  `json:"merchantCategory,omitempty"`
	Currency                string  `json:"currency,omitempty"`
	AccountType             string  `json:"accountType,omitempty"`
	Occupation              string  `json:"occupation,omitempty"`
	KYCStatus               string  `json:"kycStatus,omitempty"`
	DeviceType              string  `json:"deviceType,omitempty"`
	AuthenticationMethod    string  `json:"authenticationMethod,omitempty"`
}

type Assessment struct {
	Level        RiskLevel `json:"level"`
	Score        float64   `json:"score"`
	Reason       string    `json:"reason"`
	ModelVersion string    `json:"model_version"`
}

type RiskEngine struct {
	predictor       ModelPredictor
	heuristic       *HeuristicPredictor
	amt             config.AmountTermParams
	coldStartMonths int
	// pre is the training-time preprocessing contract (label encoders + scaler)
	// for the 32-feature model. It is nil when the artifact is absent, in which
	// case ONNX is gated off and the heuristic is used (feeding raw, unscaled
	// features to the model would silently produce wrong scores).
	pre *preprocess.Artifact
}

func NewRiskEngine() RiskEngine {
	return NewRiskEngineWithParams(config.Default().AmountTerm)
}

func NewRiskEngineWithParams(amt config.AmountTermParams) RiskEngine {
	modelPath := strings.TrimSpace(os.Getenv("FINIX_RISK_MODEL_PATH"))
	if modelPath == "" {
		modelPath = "../../models/transaction_risk_model.onnx"
	}

	csm := config.Default().ColdStartMonths

	heuristic := &HeuristicPredictor{amt: amt}
	predictor := NewONNXPredictor(modelPath)

	prePath := strings.TrimSpace(os.Getenv("FINIX_RISK_PREPROCESS_PATH"))
	if prePath == "" {
		prePath = preprocess.DefaultArtifactPath(modelPath, "transaction_preprocess.json")
	}
	pre, err := preprocess.Load(prePath)
	switch {
	case err != nil:
		slog.Info("transaction preprocess artifact unavailable; ONNX gated off, using heuristic",
			"path", prePath, "error", err)
		pre = nil
	case pre.FeatureCount() != txnFeatureCount:
		slog.Warn("transaction preprocess artifact feature-count mismatch; ONNX gated off",
			"want", txnFeatureCount, "got", pre.FeatureCount())
		pre = nil
	default:
		slog.Info("transaction preprocess artifact loaded", "path", prePath, "features", pre.FeatureCount())
	}

	return RiskEngine{predictor: predictor, heuristic: heuristic, amt: amt, coldStartMonths: csm, pre: pre}
}

// ColdStart returns true if the user's account age (in months) is below the
// cold-start threshold. During cold-start the ONNX model is skipped entirely
// and the heuristic fallback is used instead.
func (e RiskEngine) ColdStart(accountAgeMonths int) bool {
	return e.coldStartMonths > 0 && accountAgeMonths < e.coldStartMonths
}

func (e RiskEngine) Evaluate(sig RiskSignal) Assessment {
	sig = normaliseSignal(sig, e.amt)

	// ONNX is used only when BOTH the model and its preprocessing contract are
	// present, so the served feature vector is label-encoded and StandardScaled
	// exactly as at training time.
	if !sig.IsColdStart && e.pre != nil && e.predictor.IsAvailable() {
		numeric, categorical := sig.featureMaps()
		features, defaulted := e.pre.BuildVector(numeric, categorical)
		probs, err := e.predictor.Predict(features)
		if err == nil && len(probs) == 2 {
			if len(defaulted) > 0 {
				slog.Debug("onnx inference used mean-defaulted features", "count", len(defaulted))
			}
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
