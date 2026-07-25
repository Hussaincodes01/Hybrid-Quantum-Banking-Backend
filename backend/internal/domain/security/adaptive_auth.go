package security

import (
	"fmt"
	"sync"
	"time"
)

// AdaptiveAuthLevel represents the 5-tier authentication escalation per spec §3A.3 Layer 4.
//
// Tier 0: Low-risk session + Low-risk transaction → Biometric only
// Tier 1: Low-risk session + Medium-risk transaction → Biometric + on-screen acknowledgement
// Tier 2: Medium-risk session + Any transaction → Biometric + fresh OTP
// Tier 3: High-risk signals detected → Block + identity verification (biometric + OTP + challenge + manual review)
// Tier 4: Anomalous session (behavioural biometric mismatch) → Silent risk escalation + reduced limits + background alert

type AdaptiveAuthTier int

const (
	TierBiometricOnly       AdaptiveAuthTier = 0
	TierBiometricAck        AdaptiveAuthTier = 1
	TierBiometricPlusOTP    AdaptiveAuthTier = 2
	TierBlockIdentityVerify AdaptiveAuthTier = 3
	TierSilentEscalation    AdaptiveAuthTier = 4
)

type SessionRiskProfile struct {
	UserID             string
	DeviceTrustScore   float64
	BehaviouralDrift   float64
	SessionAge         time.Duration
	FailedAuthAttempts int
	LastSuccessfulAuth time.Time
	IsAnomalousSession bool
	IsTrustedDevice    bool
}

type TransactionRiskProfile struct {
	RiskScore       float64
	RiskLevel       string
	AmountPaise     int64
	IsNewRecipient  bool
	AmountVsAverage float64
}

type AuthDecision struct {
	Tier                AdaptiveAuthTier `json:"tier"`
	TierName            string           `json:"tierName"`
	RequireBiometric    bool             `json:"requireBiometric"`
	RequireOTP          bool             `json:"requireOTP"`
	RequireAck          bool             `json:"requireAck"`
	RequireManualReview bool             `json:"requireManualReview"`
	IsBlocked           bool             `json:"isBlocked"`
	SilentEscalation    bool             `json:"silentEscalation"`
	ReducedTxLimit      int64            `json:"reducedTxLimit,omitempty"`
	BackgroundAlert     string           `json:"backgroundAlert,omitempty"`
	XAIReason           string           `json:"xaiReason"`
}

type AdaptiveAuthManager struct {
	mu             sync.RWMutex
	userLimits     map[string]int64
	defaultLimit   int64
	highValueLimit int64
}

func NewAdaptiveAuthManager() *AdaptiveAuthManager {
	return &AdaptiveAuthManager{
		userLimits:     make(map[string]int64),
		defaultLimit:   100000 * 100, // ₹1,00,000
		highValueLimit: 100000 * 100, // ₹1,00,000 — above this requires manual review per spec
	}
}

// EvaluateAuthTier determines the authentication tier required for a transaction
// based on the session risk profile and transaction risk profile.
func (a *AdaptiveAuthManager) EvaluateAuthTier(session SessionRiskProfile, txn TransactionRiskProfile) AuthDecision {
	// Tier 3: High-risk signals → Block + identity verification (checked first for safety)
	if txn.RiskLevel == "high" || txn.RiskScore >= 70 {
		requireManualReview := txn.AmountPaise > a.highValueLimit
		return AuthDecision{
			Tier:                TierBlockIdentityVerify,
			TierName:            "block_identity_verify",
			RequireBiometric:    true,
			RequireOTP:          true,
			RequireManualReview: requireManualReview,
			IsBlocked:           true,
			XAIReason: fmt.Sprintf("High-risk transaction detected (risk score: %.1f). Transaction blocked. Identity verification required: biometric + OTP%s.", txn.RiskScore, func() string {
				if requireManualReview {
					return " + manual review (amount exceeds ₹1 lakh threshold)"
				}
				return ""
			}()),
		}
	}

	// Tier 4: Anomalous session (behavioural biometric mismatch)
	if session.IsAnomalousSession || session.BehaviouralDrift > 0.7 {
		return AuthDecision{
			Tier:             TierSilentEscalation,
			TierName:         "silent_escalation",
			RequireBiometric: true,
			SilentEscalation: true,
			ReducedTxLimit:   a.defaultLimit / 2, // Reduce limits by 50%
			BackgroundAlert:  fmt.Sprintf("Anomalous session detected for user %s. Behavioural drift: %.2f. Transaction limits reduced. Alert queued for registered email.", session.UserID, session.BehaviouralDrift),
			XAIReason:        "Session behaviour deviates significantly from your established pattern. Limits have been silently reduced and an alert has been sent to your registered email.",
		}
	}

	// Tier 2: Medium-risk session + Any transaction → Biometric + OTP
	if session.DeviceTrustScore < 0.7 || session.FailedAuthAttempts > 0 || session.SessionAge > 10*time.Minute {
		return AuthDecision{
			Tier:             TierBiometricPlusOTP,
			TierName:         "biometric_plus_otp",
			RequireBiometric: true,
			RequireOTP:       true,
			XAIReason:        "Medium-risk session detected. Step-up authentication required: biometric re-confirmation + fresh OTP from registered mobile.",
		}
	}

	// Tier 1: Low-risk session + Medium-risk transaction → Biometric + on-screen acknowledgement
	if txn.RiskLevel == "medium" || (txn.RiskScore >= 40 && txn.RiskScore < 70) {
		return AuthDecision{
			Tier:             TierBiometricAck,
			TierName:         "biometric_plus_ack",
			RequireBiometric: true,
			RequireAck:       true,
			XAIReason:        fmt.Sprintf("Medium-risk transaction detected (risk score: %.1f). Biometric re-confirmation and explicit on-screen acknowledgement required.", txn.RiskScore),
		}
	}

	// Tier 0: Low-risk session + Low-risk transaction → Biometric only
	return AuthDecision{
		Tier:             TierBiometricOnly,
		TierName:         "biometric_only",
		RequireBiometric: true,
		XAIReason:        "Transaction aligns with your normal behavior. Biometric authentication only — no additional friction.",
	}
}

// SetUserLimit sets a custom transaction limit for a user (used by Tier 4).
func (a *AdaptiveAuthManager) SetUserLimit(userID string, limit int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.userLimits[userID] = limit
}

// GetUserLimit returns the transaction limit for a user.
func (a *AdaptiveAuthManager) GetUserLimit(userID string) int64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if limit, ok := a.userLimits[userID]; ok {
		return limit
	}
	return a.defaultLimit
}

// ResetUserLimit resets a user's transaction limit to the default.
func (a *AdaptiveAuthManager) ResetUserLimit(userID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.userLimits, userID)
}

func (t AdaptiveAuthTier) String() string {
	switch t {
	case TierBiometricOnly:
		return "biometric_only"
	case TierBiometricAck:
		return "biometric_plus_ack"
	case TierBiometricPlusOTP:
		return "biometric_plus_otp"
	case TierBlockIdentityVerify:
		return "block_identity_verify"
	case TierSilentEscalation:
		return "silent_escalation"
	default:
		return "unknown"
	}
}
