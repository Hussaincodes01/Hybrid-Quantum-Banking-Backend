package platform

import (
	"time"

	"FINIX/backend/internal/domain/security"
)

// Payment represents a full payment record with all 22 fields from the spec
type Payment struct {
	ID                   string                       `json:"id"`
	UserID               string                       `json:"userId"`
	BeneficiaryID        string                       `json:"beneficiaryId,omitempty"`
	BeneficiaryName      string                       `json:"beneficiaryName"`
	BeneficiaryAccount   string                       `json:"beneficiaryAccount"`
	AmountPaise          int64                        `json:"amountPaise"`
	Currency             string                       `json:"currency"` // MISSING: was absent
	Method               string                       `json:"method"`   // MISSING: was "channel"
	Status               string                       `json:"status"`
	RiskLevel            string                       `json:"riskLevel"`                      // had riskLevel
	RiskScore            float64                      `json:"riskScore"`                      // had riskScore
	StartedAt            time.Time                    `json:"startedAt"`                      // MISSING: was CreatedAt
	CompletedAt          *time.Time                   `json:"completedAt,omitempty"`          // MISSING: was absent
	BankReference        string                       `json:"bankReference"`                  // MISSING: was absent
	OTPVerified          bool                         `json:"otpVerified"`                    // MISSING: was in OverrideRequest
	UserConsent          bool                         `json:"userConsent"`                    // MISSING: was absent
	RefundInfo           *RefundInfo                  `json:"refundInfo,omitempty"`           // MISSING: was absent
	PaymentIntentID      string                       `json:"paymentIntentId"`                // MISSING: was absent
	UPIIntentID          string                       `json:"upiIntentId,omitempty"`          // MISSING: was absent
	QRCodePayload        string                       `json:"qrCodePayload,omitempty"`        // MISSING: was absent
	ReceiptVerification  *ReceiptVerification         `json:"receiptVerification,omitempty"`  // MISSING
	SIPID                string                       `json:"sipId,omitempty"`                // MISSING: was absent
	PaymentTimelineID    string                       `json:"paymentTimelineId,omitempty"`    // MISSING
	StepUpChallengeID    string                       `json:"stepUpChallengeId,omitempty"`    // MISSING
	DeviceChallengeKeyID string                       `json:"deviceChallengeKeyId,omitempty"` // MISSING
	DilithiumSignature   *security.DilithiumSignature `json:"dilithiumSignature,omitempty"`   // from Phase 2
	CreatedAt            time.Time                    `json:"createdAt"`                      // was CreatedAt
}

type RefundInfo struct {
	RefundID    string     `json:"refundId"`
	AmountPaise int64      `json:"amountPaise"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status"`
	InitiatedAt time.Time  `json:"initiatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type ReceiptVerification struct {
	Verified     bool       `json:"verified"`
	VerifiedAt   *time.Time `json:"verifiedAt,omitempty"`
	VerifiedBy   string     `json:"verifiedBy,omitempty"`
	VerifierType string     `json:"verifierType,omitempty"` // "bank" | "merchant" | "user"
	ReceiptHash  string     `json:"receiptHash,omitempty"`
}
