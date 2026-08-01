package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	mrand "math/rand"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"FINIX/backend/internal/config"
	"FINIX/backend/internal/domain/blockchain"
	"FINIX/backend/internal/domain/fraud"
	"FINIX/backend/internal/domain/healthscore"
	"FINIX/backend/internal/domain/security"
	"FINIX/backend/internal/domain/transaction"
	"FINIX/backend/internal/infra/ai"
	"FINIX/backend/internal/infra/repo"
	"FINIX/backend/internal/ml/preprocess"
	"FINIX/backend/internal/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Mobile           string    `json:"mobile"`
	Email            string    `json:"email,omitempty"`
	KIN              string    `json:"kin,omitempty"`
	UBT              string    `json:"ubt"`
	EKYCVerified     bool      `json:"ekycVerified"`
	BiometricEnabled bool      `json:"biometricEnabled"`
	DeviceBound      bool      `json:"deviceBound"`
	SIMBound         bool      `json:"simBound"`
	NudgePreference  string    `json:"nudgePreference"`
	CreatedAt        time.Time `json:"createdAt"`
}

type Account struct {
	ID                        string     `json:"id"`
	AccountHolderName         string     `json:"accountHolderName"`
	BankName                  string     `json:"bankName"`
	Branch                    string     `json:"branch"`
	IFSCCode                  string     `json:"ifscCode"`
	AccountNumberMaskedToken  string     `json:"accountNumberMaskedToken"`
	UPIID                     string     `json:"upiId"`
	AccountType               string     `json:"accountType"`
	VerificationStatus        string     `json:"verificationStatus"`
	Nickname                  string     `json:"nickname"`
	PrimaryAccountFlag        bool       `json:"primaryAccountFlag"`
	AccountToken              string     `json:"accountToken"`
	TokenProvider             string     `json:"tokenProvider"`
	AccountVerificationAt     *time.Time `json:"accountVerificationTimestamp,omitempty"`
	AccountVerificationMethod string     `json:"accountVerificationMethod"`
	BalancePaise              int64      `json:"balancePaise"`
	LinkedAt                  time.Time  `json:"linkedAt"`
}

type Transaction struct {
	ID                 string                       `json:"id"`
	UserID             string                       `json:"userId"`
	AmountPaise        int64                        `json:"amountPaise"`
	Currency           string                       `json:"currency"`
	DebitCredit        string                       `json:"debitCredit"`
	Recipient          string                       `json:"recipient"`
	MerchantName       string                       `json:"merchantName"`
	MerchantCategory   string                       `json:"merchantCategory"`
	Channel            string                       `json:"channel"`
	Description        string                       `json:"description"`
	Status             string                       `json:"status"`
	AssignedCategory   string                       `json:"assignedCategory"`
	UserNotes          string                       `json:"userNotes"`
	RiskLevel          transaction.RiskLevel        `json:"riskLevel"`
	RiskScore          float64                      `json:"riskScore"`
	XAIReason          string                       `json:"xaiReason"`
	LinkedAccount      string                       `json:"linkedAccount"`
	PaymentIntentID    string                       `json:"paymentIntentId"`
	UPIIntentID        string                       `json:"upiIntentId"`
	BankReference      string                       `json:"bankReference"`
	TimelineIDs        []string                     `json:"timelineIds"`
	DilithiumSignature *security.DilithiumSignature `json:"dilithiumSignature,omitempty"`
	CreatedAt          time.Time                    `json:"createdAt"`
	IdempotencyKey     string                       `json:"idempotencyKey"`
	CoolingOffUntil    *time.Time                   `json:"coolingOffUntil,omitempty"`
	ModelVersion       string                       `json:"modelVersion,omitempty"` // Sprint 1: record model version
}

type Goal struct {
	ID                       string    `json:"id"`
	Name                     string    `json:"name"`
	Description              string    `json:"description"`
	TargetAmountPaise        int64     `json:"targetAmountPaise"`
	Currency                 string    `json:"currency"`
	SavedAmountPaise         int64     `json:"savedAmountPaise"`
	MonthlyContributionPaise int64     `json:"monthlyContributionPaise"`
	Frequency                string    `json:"frequency"`
	Priority                 string    `json:"priority"`
	StartDate                time.Time `json:"startDate"`
	TargetDate               time.Time `json:"targetDate"`
	Status                   string    `json:"status"`
	LinkedAccount            string    `json:"linkedAccount"`
	ProgressPercent          float64   `json:"progressPercent"`
	ShortfallEstimatePaise   int64     `json:"shortfallEstimatePaise"`
	CreatedAt                time.Time `json:"createdAt"`
}

type InvestmentHolding struct {
	ID                           string    `json:"id"`
	Name                         string    `json:"name"`
	InstrumentName               string    `json:"instrumentName"`
	Category                     string    `json:"category"`
	QuantityUnits                float64   `json:"quantityUnits"`
	AvgPurchasePricePaise        int64     `json:"avgPurchasePricePaise"`
	CurrentValuePaise            int64     `json:"currentValuePaise"`
	InvestedPaise                int64     `json:"investedPaise"`
	LastValuationDate            time.Time `json:"lastValuationDate"`
	SIPAmountPaise               int64     `json:"sipAmountPaise"`
	SIPFrequency                 string    `json:"sipFrequency"`
	SIPStartDate                 time.Time `json:"sipStartDate"`
	SIPStatus                    string    `json:"sipStatus"`
	BrokerFundHouse              string    `json:"brokerFundHouse"`
	DividendsPaise               int64     `json:"dividendsPaise"`
	TaxLotDate                   time.Time `json:"taxLotDate"`
	CAGR                         float64   `json:"cagr"`
	RecommendationID             string    `json:"recommendationId"`
	RecommendationExplainability string    `json:"recommendationExplainability"`
	CreatedAt                    time.Time `json:"createdAt"`
}

type InsurancePolicy struct {
	PolicyID        string `json:"policyId"`
	PolicyType      string `json:"policyType"`
	Insurer         string `json:"insurer"`
	SumAssuredPaise int64  `json:"sumAssuredPaise"`
	PremiumPaise    int64  `json:"premiumPaise"`
	NextDueDate     string `json:"nextDueDate"`
}

type LoanRecord struct {
	LoanID           string  `json:"loanId"`
	Lender           string  `json:"lender"`
	LoanType         string  `json:"loanType"`
	OutstandingPaise int64   `json:"outstandingPaise"`
	EMIPaise         int64   `json:"emiPaise"`
	InterestRate     float64 `json:"interestRate"`
	RemainingMonths  int     `json:"remainingMonths"`
}

type AuditEvent struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	UserID      string    `json:"userId"`
	EventType   string    `json:"eventType"`
	TriggeredBy string    `json:"triggeredBy"`
	Outcome     string    `json:"outcome"`
	Details     string    `json:"details"`
	XAIReason   string    `json:"xaiReason,omitempty"`
}

type FraudReport struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type NotificationSetting struct {
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
}

type RegisterRequest struct {
	Name                string `json:"name"`
	Mobile              string `json:"mobile"`
	Email               string `json:"email"`
	DeviceIDFingerprint string `json:"deviceIdFingerprint"`
	DeviceType          string `json:"deviceType"`
	AppVersion          string `json:"appVersion"`
	IPAddress           string `json:"ipAddress"`
	PIN                 string `json:"pin,omitempty"`
}

type RegisterResponse struct {
	UserID string `json:"userId"`
	UBT    string `json:"ubt"`
}

type EKYCRequest struct {
	UserID       string `json:"userId"`
	PanLast4     string `json:"panLast4"`
	AadhaarLast4 string `json:"aadhaarLast4"`
}

type BiometricSetupRequest struct {
	UserID       string `json:"userId"`
	PublicKeyB64 string `json:"publicKeyB64"`
	KeyID        string `json:"keyId,omitempty"`
	DeviceName   string `json:"deviceName,omitempty"`
}

type LoginChallengeRequest struct {
	UserID string `json:"userId"`
}

type LoginChallengeResponse struct {
	ChallengeID string `json:"challengeId"`
	Challenge   string `json:"challenge"`
}

type LoginVerifyRequest struct {
	UserID              string `json:"userId"`
	ChallengeID         string `json:"challengeId"`
	Signature           string `json:"signature"`
	KeyID               string `json:"keyId"`
	DeviceIDFingerprint string `json:"deviceIdFingerprint"`
}

type LoginVerifyResponse struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int    `json:"expiresInSeconds"`
}

type ChallengeState struct {
	ChallengeID string    `json:"challengeId"`
	NonceID     string    `json:"nonceId"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	VerifiedAt  time.Time `json:"verifiedAt,omitempty"`
}

type AuthProfile struct {
	InternalUserID             string     `json:"internalUserId"`
	FullName                   string     `json:"fullName"`
	MobileNumber               string     `json:"mobileNumber"`
	Email                      string     `json:"email"`
	PasswordHash               string     `json:"passwordHash,omitempty"`
	OTPCode                    string     `json:"otpCode,omitempty"`
	OTPExpiryTime              *time.Time `json:"otpExpiryTime,omitempty"`
	OTPAttempts                int        `json:"otpAttempts"`
	DeviceIDFingerprint        string     `json:"deviceIdFingerprint"`
	DeviceType                 string     `json:"deviceType"`
	AppVersion                 string     `json:"appVersion"`
	LastLoginTime              *time.Time `json:"lastLoginTime,omitempty"`
	IPAddress                  string     `json:"ipAddress"`
	SessionTokenID             string     `json:"sessionTokenId,omitempty"`
	SessionStartTime           *time.Time `json:"sessionStartTime,omitempty"`
	SessionExpiry              *time.Time `json:"sessionExpiry,omitempty"`
	TrustedDeviceFlag          bool       `json:"trustedDeviceFlag"`
	FailedLoginCount           int        `json:"failedLoginCount"`
	PINLockedUntil             *time.Time `json:"pinLockedUntil,omitempty"` // H-3: per-account PIN lockout
	DeviceChallengePublicKeyID string     `json:"deviceChallengePublicKeyId,omitempty"`
	ChallengeNonceID           string     `json:"challengeNonceId,omitempty"`
	ChallengeStatus            string     `json:"challengeStatus"`
	RegisteredKeyFingerprint   string     `json:"registeredKeyFingerprint,omitempty"`
	StepUpAuthRequiredFlag     bool       `json:"stepUpAuthRequiredFlag"`
	Role                       string     `json:"role"`
	// KYCVerified / BiometricEnabled mirror the User record so the profile
	// endpoint is self-contained for clients (see frontend_compat.go, which also
	// emits a `userId` alias for InternalUserID).
	KYCVerified      bool `json:"kycVerified"`
	BiometricEnabled bool `json:"biometricEnabled"`
}

func (a *AuthProfile) SanitizeForAPI() {
	a.PasswordHash = ""
	a.OTPCode = ""
	a.MobileNumber = MaskPhone(a.MobileNumber)
	a.Email = MaskEmail(a.Email)
	a.Role = ""
	a.DeviceChallengePublicKeyID = ""
	a.ChallengeNonceID = ""
	a.SessionTokenID = ""
}

type UpdateAuthProfileRequest struct {
	FullName                   string `json:"fullName"`
	Email                      string `json:"email"`
	DeviceIDFingerprint        string `json:"deviceIdFingerprint"`
	DeviceType                 string `json:"deviceType"`
	AppVersion                 string `json:"appVersion"`
	IPAddress                  string `json:"ipAddress"`
	TrustedDeviceFlag          *bool  `json:"trustedDeviceFlag"`
	DeviceChallengePublicKeyID string `json:"deviceChallengePublicKeyId"`
	RegisteredKeyFingerprint   string `json:"registeredKeyFingerprint"`
	StepUpAuthRequiredFlag     *bool  `json:"stepUpAuthRequiredFlag"`
}

type KYCProfile struct {
	UserID                string     `json:"userId"`
	FullName              string     `json:"fullName"`
	DateOfBirth           string     `json:"dateOfBirth"`
	Gender                string     `json:"gender"`
	MobileNumber          string     `json:"mobileNumber"`
	Email                 string     `json:"email"`
	PermanentAddress      string     `json:"permanentAddress"`
	CommunicationAddress  string     `json:"communicationAddress"`
	PANMasked             string     `json:"panMasked"`
	AadhaarMaskedOrHash   string     `json:"aadhaarMaskedOrHash"`
	KYCDocumentTypes      []string   `json:"kycDocumentTypes"`
	KYCDocumentFiles      []string   `json:"kycDocumentFiles"`
	KYCStatus             string     `json:"kycStatus"`
	KYCSubmissionDate     *time.Time `json:"kycSubmissionDate,omitempty"`
	Verifier              string     `json:"verifier"`
	Occupation            string     `json:"occupation"`
	Employer              string     `json:"employer"`
	AnnualIncome          string     `json:"annualIncome"`
	TaxResidency          string     `json:"taxResidency"`
	NomineeDetails        string     `json:"nomineeDetails"`
	KYCVerifierID         string     `json:"kycVerifierId"`
	KYCDocumentHashRef    string     `json:"kycDocumentHashReference"`
	KYCRejectionReason    string     `json:"kycRejectionReason"`
	KYCVerificationMethod string     `json:"kycVerificationMethod"`
}

func (k *KYCProfile) SanitizeForAPI() {
	k.MobileNumber = MaskPhone(k.MobileNumber)
	k.Email = MaskEmail(k.Email)
	k.PANMasked = MaskPAN(k.PANMasked)
	k.AadhaarMaskedOrHash = MaskAadhaar(k.AadhaarMaskedOrHash)
}

type UpsertKYCProfileRequest struct {
	FullName              string   `json:"fullName"`
	DateOfBirth           string   `json:"dateOfBirth"`
	Gender                string   `json:"gender"`
	Email                 string   `json:"email"`
	PermanentAddress      string   `json:"permanentAddress"`
	CommunicationAddress  string   `json:"communicationAddress"`
	PANMasked             string   `json:"panMasked"`
	AadhaarMaskedOrHash   string   `json:"aadhaarMaskedOrHash"`
	KYCDocumentTypes      []string `json:"kycDocumentTypes"`
	KYCDocumentFiles      []string `json:"kycDocumentFiles"`
	KYCStatus             string   `json:"kycStatus"`
	Verifier              string   `json:"verifier"`
	Occupation            string   `json:"occupation"`
	Employer              string   `json:"employer"`
	AnnualIncome          string   `json:"annualIncome"`
	TaxResidency          string   `json:"taxResidency"`
	NomineeDetails        string   `json:"nomineeDetails"`
	KYCVerifierID         string   `json:"kycVerifierId"`
	KYCDocumentHashRef    string   `json:"kycDocumentHashReference"`
	KYCRejectionReason    string   `json:"kycRejectionReason"`
	KYCVerificationMethod string   `json:"kycVerificationMethod"`
}

type LinkAccountRequest struct {
	AccountHolderName         string `json:"accountHolderName"`
	BankName                  string `json:"bankName"`
	Branch                    string `json:"branch"`
	IFSCCode                  string `json:"ifscCode"`
	MaskedAccount             string `json:"maskedAccount"`
	UPIID                     string `json:"upiId"`
	AccountType               string `json:"accountType"`
	VerificationStatus        string `json:"verificationStatus"`
	Nickname                  string `json:"nickname"`
	PrimaryAccountFlag        bool   `json:"primaryAccountFlag"`
	AccountToken              string `json:"accountToken"`
	TokenProvider             string `json:"tokenProvider"`
	AccountVerificationMethod string `json:"accountVerificationMethod"`
	BalancePaise              int64  `json:"balancePaise"`
}

type UpdateAccountRequest struct {
	Nickname                  string `json:"nickname"`
	PrimaryAccountFlag        *bool  `json:"primaryAccountFlag"`
	VerificationStatus        string `json:"verificationStatus"`
	AccountVerificationMethod string `json:"accountVerificationMethod"`
}

type Beneficiary struct {
	ID                      string     `json:"id"`
	BeneficiaryName         string     `json:"beneficiaryName"`
	RelationshipDescription string     `json:"relationshipDescription"`
	UPIIDOrBankDetails      string     `json:"upiIdOrBankDetails"`
	DateAdded               time.Time  `json:"dateAdded"`
	AddedBy                 string     `json:"addedBy"`
	VerificationStatus      string     `json:"verificationStatus"`
	TrustScore              float64    `json:"trustScore"`
	Notes                   string     `json:"notes"`
	LastPaymentDate         *time.Time `json:"lastPaymentDate,omitempty"`
	ApprovalInitiationID    string     `json:"approvalInitiationId"`
	ApproverID              string     `json:"approverId,omitempty"`
	ApprovalTimestamp       *time.Time `json:"approvalTimestamp,omitempty"`
	CoolingActionID         string     `json:"coolingActionId,omitempty"`
}

type CreateBeneficiaryRequest struct {
	BeneficiaryName         string  `json:"beneficiaryName"`
	RelationshipDescription string  `json:"relationshipDescription"`
	UPIIDOrBankDetails      string  `json:"upiIdOrBankDetails"`
	TrustScore              float64 `json:"trustScore"`
	Notes                   string  `json:"notes"`
}

type ApproveBeneficiaryRequest struct {
	ApproverID      string `json:"approverId"`
	CoolingActionID string `json:"coolingActionId"`
	Approved        bool   `json:"approved"`
}

type AggregatorStatus struct {
	LinkedInstitutionID        string     `json:"linkedInstitutionId"`
	ConnectorStatus            string     `json:"connectorStatus"`
	LastSyncTimestamp          *time.Time `json:"lastSyncTimestamp,omitempty"`
	NumberOfAccountsAggregated int        `json:"numberOfAccountsAggregated"`
}

type SyncAggregatorRequest struct {
	LinkedInstitutionID string `json:"linkedInstitutionId"`
	ConnectorStatus     string `json:"connectorStatus"`
}

type DashboardResponse struct {
	Greeting         string  `json:"greeting"`
	NetWorthPaise    int64   `json:"netWorthPaise"`
	WeekDeltaPaise   int64   `json:"weekDeltaPaise"`
	HealthScore      int     `json:"healthScore"`
	RiskBand         string  `json:"riskBand"`
	MarketSensex     float64 `json:"marketSensex"`
	MarketGoldPer10g float64 `json:"marketGoldPer10g"`
	FreezeActive     bool    `json:"freezeActive"`
}

type InitiateTransactionRequest struct {
	AmountPaise       int64   `json:"amountPaise"`
	Recipient         string  `json:"recipient"`
	Channel           string  `json:"channel"`
	SessionTrustScore float64 `json:"sessionTrustScore"`
	BehaviourDrift    float64 `json:"behaviourDrift"`
	FailedPINAttempts int     `json:"failedPinAttempts"`

	// Optional client-supplied metadata. These are commonly sent by the mobile
	// app; they are accepted (so requests carrying them are not rejected even
	// under strict JSON decoding) but are not required by the risk/debit logic.
	BeneficiaryID string `json:"beneficiaryId,omitempty"`
	AccountID     string `json:"accountId,omitempty"`
	Description   string `json:"description,omitempty"`
}

type TransactionResult struct {
	TransactionID   string                `json:"transactionId"`
	Status          string                `json:"status"`
	RiskLevel       transaction.RiskLevel `json:"riskLevel"`
	RiskScore       float64               `json:"riskScore"`
	XAIReason       string                `json:"xaiReason"`
	CoolingOffUntil *time.Time            `json:"coolingOffUntil,omitempty"`
	StepUpRequired  bool                  `json:"stepUpRequired"`
	// Step-up requirements from the adaptive-auth tier (spec §3A.3). These make
	// the required authentication explicit so the client can gate completion:
	// medium -> biometric (+ack); high -> biometric + OTP (via OverrideTransaction).
	RequiredTier        string                       `json:"requiredAuthTier,omitempty"`
	RequireBiometric    bool                         `json:"requireBiometric,omitempty"`
	RequireOTP          bool                         `json:"requireOtp,omitempty"`
	RequireManualReview bool                         `json:"requireManualReview,omitempty"`
	DilithiumSignature  *security.DilithiumSignature `json:"dilithiumSignature,omitempty"`
}

type OverrideRequest struct {
	TransactionID string `json:"transactionId"`
	OTP           string `json:"otp"`
	BiometricOK   bool   `json:"biometricOk"`
}

type GoalCreateRequest struct {
	Name                     string    `json:"name"`
	Description              string    `json:"description"`
	TargetAmountPaise        int64     `json:"targetAmountPaise"`
	Currency                 string    `json:"currency"`
	MonthlyContributionPaise int64     `json:"monthlyContributionPaise"`
	Frequency                string    `json:"frequency"`
	Priority                 string    `json:"priority"`
	StartDate                time.Time `json:"startDate"`
	TargetDate               time.Time `json:"targetDate"`
	LinkedAccount            string    `json:"linkedAccount"`
}

type GoalContributionRequest struct {
	AmountPaise int64 `json:"amountPaise"`
	// IdempotencyKey (L-9) is populated by the handler from the Idempotency-Key
	// header (the client already sends it on mutations); when present, a repeat
	// returns the first result instead of double-debiting. json:"-" so it can't be
	// spoofed via the body.
	IdempotencyKey string `json:"-"`
}

type SimulationRequest struct {
	Scenario    string `json:"scenario"`
	AmountPaise int64  `json:"amountPaise"`
	Years       int    `json:"years"`
	Iterations  int    `json:"iterations"`
}

type SimulationResult struct {
	Scenario                string  `json:"scenario"`
	ProjectedNetWorthNow    int64   `json:"projectedNetWorthNow"`
	ProjectedNetWorthFuture int64   `json:"projectedNetWorthFuture"`
	GoalCompletionDeltaPct  float64 `json:"goalCompletionDeltaPct"`
	HealthScoreDelta        int     `json:"healthScoreDelta"`
	XAIReason               string  `json:"xaiReason"`
	MonteCarloPercentiles   []int64 `json:"monteCarloPercentiles"`
	ConfidenceInterval90    []int64 `json:"confidenceInterval90"`
}

type InsightCard struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Body          string   `json:"body"`
	Reason        string   `json:"reason"`
	Action        string   `json:"action"`
	PersonaTag    string   `json:"personaTag"`
	Priority      string   `json:"priority"`
	Snoozeable    bool     `json:"snoozeable"`
	RelatedGoalID string   `json:"relatedGoalId"`
	ActionLinks   []string `json:"actionLinks"`
}

type PersonaResponse struct {
	Persona      string   `json:"persona"`
	Explanation  string   `json:"explanation"`
	ContestRoute string   `json:"contestRoute"`
	Traits       []string `json:"traits"`
	Confidence   float64  `json:"confidence"`
}

type SecurityHealth struct {
	BiometricLock         bool `json:"biometricLock"`
	DeviceBinding         bool `json:"deviceBinding"`
	SIMBinding            bool `json:"simBinding"`
	TransactionMonitoring bool `json:"transactionMonitoring"`
	SMSScannerEnabled     bool `json:"smsScannerEnabled"`
	AntiPhishingContacts  bool `json:"antiPhishingContacts"`
	// AccountFrozen reports the emergency-freeze state. It is also emitted as
	// is_frozen / account_frozen for the mobile client (see frontend_compat.go).
	AccountFrozen bool `json:"accountFrozen"`
}

type SMSScanRequest struct {
	Sender  string `json:"sender"`
	Message string `json:"message"`
}

type SMSScanResult struct {
	Badge              string   `json:"badge"`
	Reason             string   `json:"reason"`
	DeviceTrustScore   float64  `json:"deviceTrustScore"`
	RegisteredDeviceID string   `json:"registeredDeviceId"`
	TrustedDevice      bool     `json:"trustedDevice"`
	PhishingIndicators []string `json:"phishingIndicators"`
	URLScanResult      string   `json:"urlScanResult"`
	HashChainVerified  bool     `json:"hashChainVerified"`
}

type FreezeRequest struct {
	TestMode bool `json:"testMode"`
}

type UnfreezeRequest struct {
	OTP         string `json:"otp"`
	BiometricOK bool   `json:"biometricOk"`
	// VerificationMethod is the mobile client's spelling: it posts
	// {"verificationMethod":"biometric"} instead of biometricOk. Treated as
	// equivalent to BiometricOK when it names a biometric factor.
	VerificationMethod string `json:"verificationMethod,omitempty"`
}

// biometricSatisfied reports whether the request carries a biometric factor,
// under either field spelling.
func (r UnfreezeRequest) biometricSatisfied() bool {
	if r.BiometricOK {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(r.VerificationMethod)) {
	case "biometric", "face", "fingerprint":
		return true
	default:
		return false
	}
}

type FraudReportRequest struct {
	Description string `json:"description"`
}

type TaxDashboard struct {
	PANMasked                  string           `json:"panMasked"`
	FinancialYear              string           `json:"financialYear"`
	TaxableIncomePaise         int64            `json:"taxableIncomePaise"`
	IncomeBreakdown            map[string]int64 `json:"incomeBreakdown"`
	Deductions                 []Deduction      `json:"deductions"`
	TaxRegime                  string           `json:"taxRegime"`
	EstimatedTaxLiabilityPaise int64            `json:"estimatedTaxLiabilityPaise"`
	TdsDeductedPaise           int64            `json:"tdsDeductedPaise"`
	NetPayablePaise            int64            `json:"netPayablePaise"`
	ITRStatus                  string           `json:"itrStatus"`
	ITRReferenceNumber         string           `json:"itrReferenceNumber"`
	TaxFilingProvider          string           `json:"taxFilingProvider"`
	Suggestions                []string         `json:"suggestions"`
	DaysToITRDeadline          int              `json:"daysToITrDeadline"`
}

type TaxRegimeComparison struct {
	OldRegimeTaxPaise int64  `json:"oldRegimeTaxPaise"`
	NewRegimeTaxPaise int64  `json:"newRegimeTaxPaise"`
	Recommended       string `json:"recommended"`
	XAIReason         string `json:"xaiReason"`
}

type Deduction struct {
	Section        string `json:"section"`
	UsedPaise      int64  `json:"usedPaise"`
	LimitPaise     int64  `json:"limitPaise"`
	RemainingPaise int64  `json:"remainingPaise"`
	Recommendation string `json:"recommendation"`
}

type CapitalGains struct {
	STCGPaise int64  `json:"stcgPaise"`
	LTCGPaise int64  `json:"ltcgPaise"`
	Tip       string `json:"tip"`
}

type ChatRequest struct {
	Prompt string `json:"prompt"`
}

type ChatResponse struct {
	Reply           string         `json:"reply"`
	Disclaimer      string         `json:"disclaimer"`
	ContextSnapshot map[string]any `json:"contextSnapshot"`
	Suggestions     []string       `json:"suggestions"`
	Explainability  string         `json:"explainability"`
	ModelID         string         `json:"modelId"`
	ModelVersion    string         `json:"modelVersion"`
	// H-2 fix: never serialize the system prompt to clients. json:"-" keeps it
	// internal (available server-side for audit) but out of every API response,
	// so users can't enumerate prompt constraints to craft jailbreaks.
	LLMPromptSnapshot   string    `json:"-"`
	ResponseGeneratedAt time.Time `json:"responseGeneratedAt"`
	RecommendationID    string    `json:"recommendationId"`
	ModelConfidence     float64   `json:"modelConfidence"`
}

type FeedbackRequest struct {
	Category string `json:"category"`
	Message  string `json:"message"`
}

type AuditIntegrity struct {
	MerkleRoot string `json:"merkleRoot"`
	EventCount int    `json:"eventCount"`
}

type NetWorthAsset struct {
	ID              string    `json:"id"`
	Type            string    `json:"type"`
	Description     string    `json:"description"`
	ValuePaise      int64     `json:"valuePaise"`
	ValuationDate   time.Time `json:"valuationDate"`
	ValuationSource string    `json:"valuationSource"`
}

type NetWorthLiability struct {
	ID               string    `json:"id"`
	Type             string    `json:"type"`
	Description      string    `json:"description"`
	LoanAmountPaise  int64     `json:"loanAmountPaise"`
	OutstandingPaise int64     `json:"outstandingPaise"`
	EMIPaise         int64     `json:"emiPaise"`
	InterestRate     float64   `json:"interestRate"`
	MaturityDate     time.Time `json:"maturityDate"`
}

type NetWorthSnapshot struct {
	Assets                []NetWorthAsset     `json:"assets"`
	Liabilities           []NetWorthLiability `json:"liabilities"`
	TotalAssetsPaise      int64               `json:"totalAssetsPaise"`
	TotalLiabilitiesPaise int64               `json:"totalLiabilitiesPaise"`
	NetWorthPaise         int64               `json:"netWorthPaise"`
	TrendArrow            string              `json:"trendArrow"`
	ValuationDate         time.Time           `json:"valuationDate"`
}

type MarketNewsItem struct {
	ID                 string    `json:"id"`
	Source             string    `json:"source"`
	PublishTimestamp   time.Time `json:"publishTimestamp"`
	SentimentScore     float64   `json:"sentimentScore"`
	Headline           string    `json:"headline"`
	Summary            string    `json:"summary"`
	URL                string    `json:"url"`
	FreshnessTimestamp time.Time `json:"freshnessTimestamp"`
}

type PortfolioImpact struct {
	Symbol        string  `json:"symbol"`
	CurrentValue  float64 `json:"currentValue"`
	ChangePercent float64 `json:"changePercent"`
	ImpactPaise   int64   `json:"impactPaise"`
}

type MarketNarration struct {
	Text          string    `json:"text"`
	GeneratedAt   time.Time `json:"generatedAt"`
	Model         string    `json:"model"`
	KeyIndicators []string  `json:"keyIndicators"`
}

type EmergencyContact struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
}

type SecurityTip struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

type HelpArticle struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Category string `json:"category"`
}

type FeatureFlag struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

type NotificationItem struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"createdAt"`
}

type GoalDissolveRequest struct {
	RedistributeToAccountID string `json:"redistributeToAccountId,omitempty"`
}

type GoalNudgeRequest struct {
	NudgeType string `json:"nudgeType"`
}

type EngineFlowEvent struct {
	FlowRunID  string    `json:"flowRunId"`
	SSEEventID string    `json:"sseEventId"`
	EventType  string    `json:"eventType"`
	UserID     string    `json:"userId"`
	Payload    any       `json:"payload"`
	FlowState  string    `json:"flowState"`
	StepName   string    `json:"stepName"`
	Timestamp  time.Time `json:"timestamp"`
}

type SSEEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type Service struct {
	mu                 sync.RWMutex
	counter            uint64
	users              map[string]*User
	authProfiles       map[string]*AuthProfile
	kycProfiles        map[string]*KYCProfile
	bankConnections    map[string]BankConnectionStatus
	realtimeDetections map[string][]RealtimeDetectionResult
	mobileIndex        map[string]string
	sessions           map[string]string
	sessionStart       map[string]time.Time
	sessionExpiry      map[string]time.Time
	challenges         map[string]string
	challengeState     map[string]ChallengeState
	accounts           map[string][]Account
	transactions       map[string][]Transaction
	beneficiaries      map[string]map[string]struct{}
	beneficiaryRecords map[string][]Beneficiary
	aggregatorStatus   map[string]AggregatorStatus
	idempotencyResult  map[string]TransactionResult
	idempotencyExpiry  map[string]time.Time // H-10: TTL for each cached idempotent result
	goalIdempotency    map[string]Goal      // L-9: dedup goal contributions by user+goal+key
	goals              map[string][]Goal
	payments           map[string][]Payment
	investments        map[string][]InvestmentHolding
	insurance          map[string][]InsurancePolicy
	loans              map[string][]LoanRecord
	audit              []AuditEvent
	consents           map[string]map[string]bool
	freezeState        map[string]bool
	notifications      map[string][]NotificationSetting
	fraudReports       []FraudReport
	riskEngine         transaction.RiskEngine
	healthCalculator   healthscore.Calculator
	ledger             *blockchain.Ledger
	exp                *expandedState
	chatbot            *ChatbotClient
	ragClient          *ai.RagClient
	passkey            *security.PasskeyManager
	sessionDevice      map[string]string
	dilithium          *security.DilithiumManager
	simBinding         *security.SIMBindingManager

	// Swappable integration providers (mock by default, real by env). Injected
	// at the composition root via UseBankAdapter/UseKYCProvider/UseSIMVerifier;
	// nil = fall back to the legacy in-process behaviour. See bank_adapter.go.
	bank BankAdapter
	kyc  security.KYCProvider
	sim  security.SIMVerifier

	// jwt, when injected (composition root), makes LoginWithPIN mint short-lived
	// signed JWTs instead of opaque tokens (spec §3.6). nil = opaque tokens, so
	// direct-NewService() unit tests keep working. See jwt.go.
	jwt                 *security.JWTManager
	adaptiveAuth        *security.AdaptiveAuthManager
	threatResponse      *security.ThreatResponseManager
	fraudGraph          *fraud.FraudGraph
	mulePredictor       fraud.GraphPredictor
	muleScores          map[string]float64
	db                  *ServicePostgres
	netWorthAssets      map[string][]NetWorthAsset
	netWorthLiabilities map[string][]NetWorthLiability
	marketNews          []MarketNewsItem
	emergencyContacts   map[string][]EmergencyContact
	featureFlags        map[string][]FeatureFlag
	notificationCenter  map[string][]NotificationItem
	sseSubscribers      map[string][]chan SSEEvent
	sseMu               sync.RWMutex
	notifMu             sync.Mutex
	accountingLedger    *AccountingLedger
	secretsManager      *SecretsManager
	outbox              *Outbox
	// outboxCancel stops the single background outbox worker on Close so the
	// goroutine terminates on shutdown instead of leaking.
	outboxCancel context.CancelFunc
	// outboxPublisher, when set, delivers an outbox event downstream; a non-nil
	// error triggers retry/dead-letter instead of silently dropping (M-12).
	outboxPublisher  func(*OutboxEvent) error
	dbCircuitBreaker *CircuitBreaker
	oauthServer      *OAuthServer
	sessionStore     SessionStore

	// params is the versioned ModelParams (spec "Srishti Math") — the single
	// source for every threshold/weight/rate. Defaults to config.Default(); the
	// composition root may override via UseModelParams.
	params config.ModelParams
}

func NewService() *Service {
	svc := &Service{
		users:               make(map[string]*User),
		authProfiles:        make(map[string]*AuthProfile),
		kycProfiles:         make(map[string]*KYCProfile),
		bankConnections:     make(map[string]BankConnectionStatus),
		realtimeDetections:  make(map[string][]RealtimeDetectionResult),
		mobileIndex:         make(map[string]string),
		sessions:            make(map[string]string),
		sessionStart:        make(map[string]time.Time),
		sessionExpiry:       make(map[string]time.Time),
		challenges:          make(map[string]string),
		challengeState:      make(map[string]ChallengeState),
		accounts:            make(map[string][]Account),
		transactions:        make(map[string][]Transaction),
		beneficiaries:       make(map[string]map[string]struct{}),
		beneficiaryRecords:  make(map[string][]Beneficiary),
		aggregatorStatus:    make(map[string]AggregatorStatus),
		idempotencyResult:   make(map[string]TransactionResult),
		idempotencyExpiry:   make(map[string]time.Time),
		goalIdempotency:     make(map[string]Goal),
		goals:               make(map[string][]Goal),
		payments:            make(map[string][]Payment),
		investments:         make(map[string][]InvestmentHolding),
		insurance:           make(map[string][]InsurancePolicy),
		loans:               make(map[string][]LoanRecord),
		audit:               make([]AuditEvent, 0, 1024),
		consents:            make(map[string]map[string]bool),
		freezeState:         make(map[string]bool),
		notifications:       make(map[string][]NotificationSetting),
		fraudReports:        make([]FraudReport, 0, 128),
		riskEngine:          transaction.NewRiskEngine(),
		healthCalculator:    healthscore.NewCalculator(),
		ledger:              blockchain.NewLedger(),
		exp:                 newExpandedState(),
		secretsManager:      NewSecretsManager(),
		chatbot:             NewChatbotClient(),
		passkey:             security.NewPasskeyManager(),
		sessionDevice:       make(map[string]string),
		simBinding:          security.NewSIMBindingManager(),
		adaptiveAuth:        security.NewAdaptiveAuthManager(),
		threatResponse:      security.NewThreatResponseManager(),
		fraudGraph:          fraud.NewFraudGraph(),
		mulePredictor:       fraud.NewMuleONNXPredictor(envOrDefaultMulePath()),
		muleScores:          make(map[string]float64),
		netWorthAssets:      make(map[string][]NetWorthAsset),
		netWorthLiabilities: make(map[string][]NetWorthLiability),
		marketNews:          make([]MarketNewsItem, 0),
		emergencyContacts:   make(map[string][]EmergencyContact),
		featureFlags:        make(map[string][]FeatureFlag),
		notificationCenter:  make(map[string][]NotificationItem),
		sseSubscribers:      make(map[string][]chan SSEEvent),
		accountingLedger:    NewAccountingLedger(),
		outbox:              NewOutbox(),
		dbCircuitBreaker:    NewCircuitBreaker(5, 3, 30*time.Second),
		oauthServer:         NewOAuthServer(),
		sessionStore:        NewInMemorySessionStore(),
		params:              config.Default(),
	}
	wireMuleEncoder()
	// Single background outbox worker. It owns a cancellable context so Close()
	// can stop it cleanly instead of leaking the goroutine.
	outboxCtx, cancelOutbox := context.WithCancel(context.Background())
	svc.outboxCancel = cancelOutbox
	go svc.startOutboxWorker(outboxCtx)
	return svc
}

// coolingOffWindow is how long a blocked transaction locks further payments,
// from the versioned model parameters (§6.3 base 240 min, overridable with
// FINIX_COOLOFF_BASE_MINUTES). Falls back to the 4h spec base if unset.
func (s *Service) coolingOffWindow() time.Duration {
	minutes := s.params.CoolingOff.BaseMinutes
	if minutes <= 0 {
		minutes = 240
	}
	return time.Duration(minutes) * time.Minute
}

// Close stops the background outbox worker. It is safe to call more than once.
func (s *Service) Close() {
	if s != nil && s.outboxCancel != nil {
		s.outboxCancel()
	}
}

// UseModelParams overrides the versioned model parameters (composition root).
// Validated by the caller; a zero-value Version is rejected in favour of the
// existing set so the service never runs on an empty parameter block.
func (s *Service) UseModelParams(p config.ModelParams) {
	if strings.TrimSpace(p.Version) == "" {
		return
	}
	s.mu.Lock()
	s.params = p
	s.mu.Unlock()
}

// ModelParams returns the active versioned parameter set.
func (s *Service) ModelParams() config.ModelParams {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.params
}

// startOutboxWorker is the single poller that drains the outbox and publishes
// each event downstream (Redis Streams via the wired outboxPublisher). It stops
// when ctx is cancelled (Close), so it does not leak on shutdown. There must be
// exactly one of these per Service — the previous api.startOutboxWorker in the
// router was a duplicate that both leaked (it selected on context.Background().
// Done(), which never fires) and competed for the same outbox.
func (s *Service) startOutboxWorker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox worker shutting down")
			return
		case <-ticker.C:
			events := s.outbox.Dequeue(50)
			for _, ev := range events {
				// M-12 fix: only mark an event processed when its downstream publish
				// actually succeeds. A failure goes through MarkFailed, which retries up
				// to 3 times then dead-letters — previously every event was marked
				// processed unconditionally, silently dropping anything that failed.
				if err := s.publishOutboxEvent(ev); err != nil {
					s.outbox.MarkFailed(ev.ID)
					slog.Warn("outbox publish failed, will retry", "event", ev.ID, "topic", ev.Topic, "error", err)
					continue
				}
				s.outbox.MarkProcessed(ev.ID)
			}
			s.outbox.Purge(6 * time.Hour) // drop long-processed events so the slice can't grow unbounded
			s.pruneIdempotency()          // H-10: evict expired/oversized idempotency entries
		}
	}
}

// publishOutboxEvent delivers one outbox event to its downstream (Kafka/Redis in
// a full deployment). When no publisher is wired it is a successful no-op; a real
// publisher returns an error on failure so the worker retries instead of dropping.
func (s *Service) publishOutboxEvent(ev *OutboxEvent) error {
	if s.outboxPublisher == nil {
		return nil
	}
	return s.outboxPublisher(ev)
}

// idempotency cache bounds (H-10): entries expire after the TTL and the map is
// hard-capped so a flood of unique Idempotency-Key headers can't exhaust memory.
const (
	idempotencyTTL        = 24 * time.Hour
	idempotencyMaxEntries = 10000
)

// setIdempotentLocked stores a result under its TTL and prunes. Caller holds s.mu.
func (s *Service) setIdempotentLocked(key string, res TransactionResult) {
	s.idempotencyResult[key] = res
	s.idempotencyExpiry[key] = time.Now().UTC().Add(idempotencyTTL)
	s.pruneIdempotencyLocked()
}

// pruneIdempotency evicts expired entries (and, if still over the cap, the
// soonest-to-expire ones) under the service lock.
func (s *Service) pruneIdempotency() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneIdempotencyLocked()
}

// pruneIdempotencyLocked is the lock-held body. Caller holds s.mu.
func (s *Service) pruneIdempotencyLocked() {
	now := time.Now().UTC()
	for k, exp := range s.idempotencyExpiry {
		if now.After(exp) {
			delete(s.idempotencyResult, k)
			delete(s.idempotencyExpiry, k)
		}
	}
	// Hard cap: if still oversized, evict the entries closest to expiry.
	for len(s.idempotencyResult) > idempotencyMaxEntries {
		var oldestKey string
		var oldest time.Time
		first := true
		for k, exp := range s.idempotencyExpiry {
			if first || exp.Before(oldest) {
				oldestKey, oldest, first = k, exp, false
			}
		}
		if oldestKey == "" {
			break
		}
		delete(s.idempotencyResult, oldestKey)
		delete(s.idempotencyExpiry, oldestKey)
	}
}

func (s *Service) IsDBConnected() bool {
	return s.db != nil
}

func (s *Service) SetDilithiumManager(dm *security.DilithiumManager) {
	s.dilithium = dm
}

func (s *Service) SignWithDilithium(data []byte) (*security.DilithiumSignature, error) {
	if s.dilithium == nil {
		return nil, errors.New("dilithium manager not initialized")
	}
	return s.dilithium.Sign(data)
}

func (s *Service) VerifyDilithiumSignature(sig *security.DilithiumSignature, data []byte) (bool, error) {
	if s.dilithium == nil {
		return false, errors.New("dilithium manager not initialized")
	}
	return s.dilithium.Verify(sig, data)
}

func NewServiceWithDB(pool *pgxpool.Pool) *Service {
	svc := NewService()
	svc.db = NewServicePostgres(repo.New(pool))
	return svc
}

// UseFabricLedger makes Hyperledger Fabric the system of record for the event
// ledger. Every WriteEvent then becomes a chaincode transaction whose smart
// contracts (cooling-off, consent revocation, double-spend) are enforced
// on-chain. With no backend attached the in-memory ledger is used instead, so
// the service still runs without the Fabric network.
func (s *Service) UseFabricLedger(backend blockchain.FabricBackend) {
	if s == nil || s.ledger == nil || backend == nil {
		return
	}
	s.ledger.UseFabric(backend)
}

// LedgerIsFabricBacked reports whether the ledger is writing to Fabric.
func (s *Service) LedgerIsFabricBacked() bool {
	return s != nil && s.ledger != nil && s.ledger.FabricEnabled()
}

// UseRagClient injects the RAG client for AI ecosystem integration.
// When set, Chat() and ScanSMS() delegate to the Python FINIX RAG service.
// Pass nil to fall back to local chatbot behavior.
func (s *Service) UseRagClient(client *ai.RagClient) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ragClient = client
}

// Outbox returns the service's outbox for event publishing.
func (s *Service) Outbox() *Outbox {
	if s == nil {
		return nil
	}
	return s.outbox
}

// SetOutboxPublisher sets the function to publish outbox events downstream.
// This is called from the composition root (main.go) with the Redis client.
func (s *Service) SetOutboxPublisher(publisher func(*OutboxEvent) error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outboxPublisher = publisher
}

func (s *Service) Register(req RegisterRequest) (RegisterResponse, error) {
	req.Mobile = strings.TrimSpace(req.Mobile)
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.DeviceIDFingerprint = strings.TrimSpace(req.DeviceIDFingerprint)
	req.DeviceType = strings.TrimSpace(req.DeviceType)
	req.AppVersion = strings.TrimSpace(req.AppVersion)
	req.IPAddress = strings.TrimSpace(req.IPAddress)
	if req.Mobile == "" {
		return RegisterResponse{}, errors.New("mobile number is required")
	}
	if req.DeviceIDFingerprint == "" {
		return RegisterResponse{}, errors.New("device fingerprint is required")
	}
	if req.Name == "" {
		req.Name = "FINIX User"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.mobileIndex[req.Mobile]; ok {
		user := s.users[existingID]
		return RegisterResponse{UserID: user.ID, UBT: user.UBT}, nil
	}

	userID := "usr_" + uuid.NewString()
	ubt := "ubt_" + randomHex(10)
	now := time.Now().UTC()

	// Create SIM binding via the security manager
	simHash := security.HashSIMICCID(req.Mobile + ":" + req.DeviceIDFingerprint)
	simBound := false
	// External SIM/VNN confirmation via the injected provider (mock by default).
	// Best-effort: an unwired real provider (stub error) does not block sign-up.
	externalSIMOK := true
	if v := s.simVerifier(); v != nil {
		if chID, cerr := v.CreateChallenge(context.Background(), req.Mobile); cerr == nil {
			if ok, verr := v.VerifyChallenge(context.Background(), req.Mobile, chID, "vnn"); verr == nil {
				externalSIMOK = ok
			}
		}
	}
	if externalSIMOK {
		if ch, err := s.simBinding.CreateBindingChallenge(userID, req.Mobile, req.DeviceIDFingerprint); err == nil {
			if record, err := s.simBinding.VerifyBindingChallenge(ch.ChallengeID, ch.VNNCode, simHash); err == nil {
				ubt = record.UBT
				simBound = true
			}
		}
	}

	user := &User{
		ID:               userID,
		Name:             req.Name,
		Mobile:           req.Mobile,
		Email:            req.Email,
		UBT:              ubt,
		EKYCVerified:     false,
		BiometricEnabled: false,
		DeviceBound:      req.DeviceIDFingerprint != "",
		SIMBound:         simBound,
		NudgePreference:  "moderate",
		CreatedAt:        now,
	}
	s.users[userID] = user
	pinHash := ""
	if req.PIN != "" {
		if h, err := HashPIN(req.PIN); err == nil {
			pinHash = h
		}
	}
	s.authProfiles[userID] = &AuthProfile{
		InternalUserID:         userID,
		FullName:               req.Name,
		MobileNumber:           EncryptField(req.Mobile),
		Email:                  EncryptField(req.Email),
		DeviceIDFingerprint:    req.DeviceIDFingerprint,
		DeviceType:             firstNonEmpty(req.DeviceType, "android"),
		AppVersion:             firstNonEmpty(req.AppVersion, "0.1.0"),
		IPAddress:              req.IPAddress,
		TrustedDeviceFlag:      true,
		ChallengeStatus:        "not_created",
		StepUpAuthRequiredFlag: false,
		Role:                   "customer",
		PasswordHash:           pinHash,
	}
	s.kycProfiles[userID] = &KYCProfile{
		UserID:                userID,
		FullName:              req.Name,
		MobileNumber:          req.Mobile,
		Email:                 req.Email,
		KYCStatus:             "pending",
		KYCVerificationMethod: "manual",
	}
	s.mobileIndex[req.Mobile] = userID
	s.accounts[userID] = []Account{{
		ID:                        s.nextID("acct"),
		AccountHolderName:         req.Name,
		BankName:                  "PSB Demo Bank",
		Branch:                    "Main Branch",
		IFSCCode:                  "PSBD0001001",
		AccountNumberMaskedToken:  TokenizeAccount(s.nextID("acct"), "XXXXXX1001"),
		UPIID:                     strings.ToLower(strings.ReplaceAll(req.Name, " ", "")) + "@psb",
		AccountType:               "savings",
		VerificationStatus:        "verified",
		Nickname:                  "Primary Savings",
		PrimaryAccountFlag:        true,
		AccountToken:              "accttok_" + randomHex(8),
		TokenProvider:             "bank",
		AccountVerificationAt:     &now,
		AccountVerificationMethod: "upi",
		BalancePaise:              8500000,
		LinkedAt:                  now,
	}}
	s.beneficiaries[userID] = make(map[string]struct{})
	s.beneficiaryRecords[userID] = []Beneficiary{}
	s.aggregatorStatus[userID] = AggregatorStatus{
		LinkedInstitutionID:        "",
		ConnectorStatus:            "connected",
		LastSyncTimestamp:          &now,
		NumberOfAccountsAggregated: len(s.accounts[userID]),
	}
	s.consents[userID] = map[string]bool{
		"analytics":         true,
		"market_data":       true,
		"notifications":     true,
		"sms_scanning":      false,
		"model_improvement": true,
	}
	s.notifications[userID] = []NotificationSetting{
		{Category: "transactions", Enabled: true},
		{Category: "security", Enabled: true},
		{Category: "goals", Enabled: true},
		{Category: "insights", Enabled: true},
		{Category: "tax", Enabled: true},
	}
	s.seedPortfolio(userID)
	s.appendAuditLocked(userID, "registration_completed", "system", "success", "User registration and SIM binding completed", "System")
	if s.db != nil {
		savedUser := *user
		savedAcct := s.accounts[userID][0]
		savedConsents := make(map[string]bool)
		for k, v := range s.consents[userID] {
			savedConsents[k] = v
		}
		db := s.db
		go func() {
			ctx := context.Background()
			_ = db.SaveUserToDB(ctx, savedUser)
			_ = db.SaveAccountToDB(ctx, savedAcct, userID)
			for ct, g := range savedConsents {
				_ = db.SaveConsentToDB(ctx, userID, ct, g)
			}
		}()
	}
	return RegisterResponse{UserID: userID, UBT: ubt}, nil
}

func (s *Service) VerifyEKYC(req EKYCRequest) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[req.UserID]
	if !ok {
		return User{}, errors.New("user not found")
	}

	if len(strings.TrimSpace(req.PanLast4)) != 4 || len(strings.TrimSpace(req.AadhaarLast4)) != 4 {
		return User{}, errors.New("invalid KYC details")
	}

	// Real format validation via security package
	if err := security.VerifyPANLast4(req.PanLast4); err != nil {
		return User{}, err
	}
	if err := security.VerifyAadhaarLast4(req.AadhaarLast4); err != nil {
		return User{}, err
	}

	user.EKYCVerified = true
	// Generate KIN via the KYC provider (mock by default; a real NSDL/CKYC
	// provider can be swapped in by env). Falls back to the local CKYC-style
	// algorithm if the provider is unset or errors.
	user.KIN = security.GenerateKIN(req.AadhaarLast4, req.PanLast4)
	if s.kyc != nil {
		if kin, err := s.kyc.GenerateKIN(req.AadhaarLast4, req.PanLast4); err == nil && kin != "" {
			user.KIN = kin
		}
	}
	now := time.Now().UTC()
	profile, ok := s.kycProfiles[user.ID]
	if !ok {
		profile = &KYCProfile{UserID: user.ID, FullName: user.Name, MobileNumber: user.Mobile, Email: user.Email}
		s.kycProfiles[user.ID] = profile
	}
	profile.PANMasked = "XXXXXX" + strings.ToUpper(req.PanLast4)
	profile.AadhaarMaskedOrHash = "XXXXXXXX" + req.AadhaarLast4
	profile.KYCStatus = "verified"
	profile.KYCSubmissionDate = &now
	profile.KYCVerificationMethod = firstNonEmpty(profile.KYCVerificationMethod, "offline_xml+nsdl")
	profile.KYCVerifierID = firstNonEmpty(profile.KYCVerifierID, "system-verifier")
	s.appendAuditLocked(user.ID, "ekyc_verified", "kyc", "success", "Aadhaar offline XML + PAN verification successful", "System")
	return *user, nil
}

func (s *Service) RegisterBiometric(req BiometricSetupRequest) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[req.UserID]
	if !ok || !user.EKYCVerified {
		return User{}, errors.New("biometric setup failed")
	}
	if strings.TrimSpace(req.PublicKeyB64) == "" {
		return User{}, errors.New("public key is required")
	}

	publicKeyBytes, err := base64.StdEncoding.DecodeString(req.PublicKeyB64)
	if err != nil {
		return User{}, errors.New("invalid base64 public key encoding")
	}

	keyID := strings.TrimSpace(req.KeyID)
	if keyID == "" {
		keyID = "key_" + randomHex(8)
	}
	deviceName := req.DeviceName
	if deviceName == "" {
		deviceName = user.Name
	}
	if deviceName == "" {
		deviceName = "unknown"
	}
	if err := s.passkey.Register(user.ID, keyID, deviceName, publicKeyBytes); err != nil {
		return User{}, fmt.Errorf("failed to register passkey: %w", err)
	}

	user.BiometricEnabled = true
	if authProfile, ok := s.authProfiles[user.ID]; ok {
		authProfile.DeviceChallengePublicKeyID = keyID
		authProfile.RegisteredKeyFingerprint = shortenFingerprint(req.PublicKeyB64)
	}
	s.appendAuditLocked(user.ID, "biometric_setup", "security", "success", "Device-bound biometric key registered with real public key", "User")
	return *user, nil
}

func (s *Service) CreateChallenge(req LoginChallengeRequest) (LoginChallengeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[req.UserID]
	if !ok || !user.BiometricEnabled {
		return LoginChallengeResponse{}, errors.New("authentication failed")
	}

	ch, err := s.passkey.CreateChallenge(user.ID)
	if err != nil {
		return LoginChallengeResponse{}, fmt.Errorf("failed to create challenge: %w", err)
	}

	s.challenges[user.ID] = ch.ChallengeID
	now := time.Now().UTC()
	s.challengeState[user.ID] = ChallengeState{
		ChallengeID: ch.ChallengeID,
		NonceID:     "nonce_" + randomHex(8),
		Status:      "created",
		CreatedAt:   now,
	}
	if profile, ok := s.authProfiles[user.ID]; ok {
		profile.ChallengeNonceID = s.challengeState[user.ID].NonceID
		profile.ChallengeStatus = "created"
	}

	return LoginChallengeResponse{
		ChallengeID: ch.ChallengeID,
		Challenge:   base64.StdEncoding.EncodeToString(ch.Challenge),
	}, nil
}

func (s *Service) VerifyLogin(req LoginVerifyRequest) (LoginVerifyResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[req.UserID]
	if !ok {
		return LoginVerifyResponse{}, errors.New("verification failed")
	}

	storedChallengeID, ok := s.challenges[user.ID]
	if !ok {
		return LoginVerifyResponse{}, errors.New("verification failed")
	}
	if storedChallengeID != req.ChallengeID {
		return LoginVerifyResponse{}, errors.New("verification failed")
	}

	if strings.TrimSpace(req.Signature) == "" {
		return LoginVerifyResponse{}, errors.New("signature is required")
	}
	if strings.TrimSpace(req.KeyID) == "" {
		return LoginVerifyResponse{}, errors.New("keyId is required")
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		return LoginVerifyResponse{}, errors.New("invalid base64 signature encoding")
	}

	valid, err := s.passkey.VerifyChallenge(user.ID, req.ChallengeID, signatureBytes, req.KeyID)
	if err != nil || !valid {
		s.appendAuditLocked(user.ID, "login_failed", "auth", "failure", "Biometric signature verification failed: "+fmt.Sprintf("%v", err), "System")
		if profile, ok := s.authProfiles[user.ID]; ok {
			profile.FailedLoginCount++
			if profile.FailedLoginCount >= 10 {
				s.threatResponse.RespondToThreat(security.ThreatCredentialStuffing, user.ID, map[string]any{
					"failedAttempts": profile.FailedLoginCount,
				})
			}
		}
		return LoginVerifyResponse{}, errors.New("biometric signature verification failed")
	}

	// Device binding enforcement — no bypass
	suppliedFingerprint := strings.TrimSpace(req.DeviceIDFingerprint)
	if suppliedFingerprint == "" {
		s.appendAuditLocked(user.ID, "login_failed", "auth", "failure", "Empty device fingerprint supplied", "System")
		return LoginVerifyResponse{}, errors.New("device fingerprint is required")
	}
	if profile, ok := s.authProfiles[user.ID]; ok {
		expectedFingerprint := strings.TrimSpace(profile.DeviceIDFingerprint)
		if expectedFingerprint == "" {
			s.appendAuditLocked(user.ID, "login_failed", "auth", "failure", "Device binding not configured", "System")
			return LoginVerifyResponse{}, errors.New("device binding not configured, please re-register")
		}
		if expectedFingerprint != suppliedFingerprint {
			s.appendAuditLocked(user.ID, "login_failed", "auth", "failure", "Device binding mismatch detected", "System")
			return LoginVerifyResponse{}, errors.New("login restricted: device binding mismatch")
		}
	}

	now := time.Now().UTC()
	expiry := now.Add(15 * time.Minute)
	token := "swt_" + randomHex(20)
	s.sessions[token] = user.ID
	s.sessionStart[token] = now
	s.sessionExpiry[token] = expiry
	s.sessionDevice[token] = suppliedFingerprint
	// Write-through to session store for persistence across restarts
	s.sessionStore.Set("session:"+token, user.ID, 15*time.Minute)
	s.sessionStore.Set("session_device:"+token, suppliedFingerprint, 15*time.Minute)
	delete(s.challenges, user.ID)
	if state, ok := s.challengeState[user.ID]; ok {
		state.Status = "verified"
		state.VerifiedAt = now
		s.challengeState[user.ID] = state
	}
	if profile, ok := s.authProfiles[user.ID]; ok {
		profile.LastLoginTime = &now
		profile.SessionTokenID = token
		profile.SessionStartTime = &now
		profile.SessionExpiry = &expiry
		profile.ChallengeStatus = "verified"
		profile.ChallengeNonceID = ""
		profile.FailedLoginCount = 0
	}
	s.appendAuditLocked(user.ID, "login_success", "auth", "success", "Biometric login verified with ECDSA device-bound signature", "User")
	return LoginVerifyResponse{AccessToken: token, ExpiresIn: 900}, nil
}

func (s *Service) AuthenticateToken(token, deviceFingerprint string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	uid, ok := s.sessions[token]
	if !ok {
		// Fallback: check session store for sessions persisted across restarts
		if storedUID, err := s.sessionStore.Get("session:" + token); err == nil && storedUID != "" {
			// C-2 fix: restore the ORIGINAL expiry from the store, not a fresh 15m window.
			// A fresh window would reset TTL on every request, making sessions never expire.
			expiryKey := "session_expiry:" + token
			uid = storedUID
			ok = true
			s.sessions[token] = uid
			s.sessionStart[token] = time.Now()
			if expiryStr, err2 := s.sessionStore.Get(expiryKey); err2 == nil && expiryStr != "" {
				if t, err3 := time.Parse(time.RFC3339, expiryStr); err3 == nil {
					s.sessionExpiry[token] = t
				} else {
					s.sessionExpiry[token] = time.Now() // treat as already expired
				}
			} else {
				s.sessionExpiry[token] = time.Now() // no expiry stored → treat as expired
			}
		}
	}
	if !ok {
		return "", false
	}
	expiry, ok := s.sessionExpiry[token]
	if !ok {
		return "", false
	}
	if time.Now().After(expiry) {
		delete(s.sessions, token)
		delete(s.sessionStart, token)
		delete(s.sessionExpiry, token)
		delete(s.sessionDevice, token)
		if profile, exists := s.authProfiles[uid]; exists && profile.SessionTokenID == token {
			profile.SessionTokenID = ""
			profile.SessionStartTime = nil
			profile.SessionExpiry = nil
			profile.StepUpAuthRequiredFlag = true
		}
		return "", false
	}
	// Device binding check — if session has a bound device, the request must match
	if boundDevice, hasDevice := s.sessionDevice[token]; hasDevice && boundDevice != "" {
		if strings.TrimSpace(deviceFingerprint) == "" {
			return "", false
		}
		if deviceFingerprint != boundDevice {
			return "", false
		}
	}
	return uid, true
}

// AuthenticateTokenLegacy is kept for backward compatibility with tests that don't send device fingerprint.
// It performs authentication without device binding check (used only in test helpers).
func (s *Service) AuthenticateTokenLegacy(token string) (string, bool) {
	return s.AuthenticateToken(token, "")
}

// invalidateAllSessionsLocked removes every session belonging to userID from the
// in-memory maps and the session store, and revokes the corresponding JWTs when a
// JWT manager is configured. Caller must hold s.mu. Used on security-sensitive
// events such as a PIN change (M-10).
func (s *Service) invalidateAllSessionsLocked(userID string) {
	for token, uid := range s.sessions {
		if uid != userID {
			continue
		}
		if s.jwt != nil {
			s.jwt.RevokeTokenString(token)
		}
		delete(s.sessions, token)
		delete(s.sessionStart, token)
		delete(s.sessionExpiry, token)
		delete(s.sessionDevice, token)
		if s.sessionStore != nil {
			_ = s.sessionStore.Delete("session:" + token)
			_ = s.sessionStore.Delete("session_device:" + token)
			_ = s.sessionStore.Delete("session_expiry:" + token)
		}
	}
	if profile, ok := s.authProfiles[userID]; ok {
		profile.SessionTokenID = ""
		profile.SessionStartTime = nil
		profile.SessionExpiry = nil
		profile.StepUpAuthRequiredFlag = true
	}
}

func (s *Service) GetRole(userID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if profile, ok := s.authProfiles[userID]; ok && profile.Role != "" {
		return profile.Role
	}
	return "customer"
}

func (s *Service) ExchangeOAuthToken(req TokenRequest) (*TokenResponse, error) {
	return s.oauthServer.ExchangeToken(req)
}

func (s *Service) GenerateOAuthCode(userID, codeChallenge string) string {
	return s.oauthServer.GenerateAuthCode(userID, codeChallenge)
}

func (s *Service) LinkAccount(userID string, req LinkAccountRequest) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[userID]; !ok {
		return Account{}, errors.New("user not found")
	}
	// C-6 fix: ignore client-supplied BalancePaise entirely. A client must never
	// be able to set their own balance. The balance is populated by the bank adapter
	// on the next sync. The old >= 0 check was the only guard, which allowed
	// arbitrary inflation via {balancePaise: 999999999999}.
	if strings.TrimSpace(req.BankName) == "" || strings.TrimSpace(req.MaskedAccount) == "" {
		return Account{}, errors.New("bank name and masked account are required")
	}
	accountType := req.AccountType
	if accountType == "" {
		accountType = "savings"
	}
	now := time.Now().UTC()
	verificationStatus := firstNonEmpty(req.VerificationStatus, "pending")

	acct := Account{
		ID:                        s.nextID("acct"),
		AccountHolderName:         firstNonEmpty(req.AccountHolderName, s.users[userID].Name),
		BankName:                  req.BankName,
		Branch:                    firstNonEmpty(req.Branch, "NA"),
		IFSCCode:                  strings.ToUpper(firstNonEmpty(req.IFSCCode, "NA")),
		AccountNumberMaskedToken:  req.MaskedAccount,
		UPIID:                     req.UPIID,
		AccountType:               accountType,
		VerificationStatus:        verificationStatus,
		Nickname:                  req.Nickname,
		PrimaryAccountFlag:        req.PrimaryAccountFlag,
		AccountToken:              firstNonEmpty(req.AccountToken, "accttok_"+randomHex(8)),
		TokenProvider:             firstNonEmpty(req.TokenProvider, "bank"),
		AccountVerificationAt:     &now,
		AccountVerificationMethod: firstNonEmpty(req.AccountVerificationMethod, "upi"),
		BalancePaise:              0, // C-6: never trust client; populated by bank sync
		LinkedAt:                  now,
	}
	if acct.PrimaryAccountFlag {
		for i := range s.accounts[userID] {
			s.accounts[userID][i].PrimaryAccountFlag = false
		}
	}
	s.accounts[userID] = append(s.accounts[userID], acct)
	agg := s.aggregatorStatus[userID]
	agg.NumberOfAccountsAggregated = len(s.accounts[userID])
	nowSync := time.Now().UTC()
	agg.LastSyncTimestamp = &nowSync
	if agg.ConnectorStatus == "" {
		agg.ConnectorStatus = "connected"
	}
	s.aggregatorStatus[userID] = agg
	s.appendAuditLocked(userID, "account_linked", "account_aggregator", "success", "New account linked successfully", "User")
	if s.db != nil {
		savedAcct := acct
		db := s.db
		go func() {
			_ = db.SaveAccountToDB(context.Background(), savedAcct, userID)
		}()
	}
	return acct, nil
}

func (s *Service) Dashboard(userID string) (DashboardResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return DashboardResponse{}, errors.New("user not found")
	}

	netWorth := s.netWorthPaiseLocked(userID)
	health := s.healthScoreLocked(userID)

	return DashboardResponse{
		Greeting:         fmt.Sprintf("Good morning, %s", user.Name),
		NetWorthPaise:    netWorth,
		WeekDeltaPaise:   int64(float64(netWorth) * 0.012),
		HealthScore:      health.Score300To900,
		RiskBand:         health.Band,
		MarketSensex:     75180.42,
		MarketGoldPer10g: 74250.00,
		FreezeActive:     s.freezeState[userID],
	}, nil
}

func (s *Service) InitiateTransaction(userID, idempotencyKey string, req InitiateTransactionRequest) (TransactionResult, error) {
	// M-5: financial persistence runs synchronously as a write-through, but AFTER
	// the lock is released. Defers are LIFO, so registering this before the Unlock
	// defer makes it fire after the unlock — the transaction (and, on a committed
	// debit, the new account balances) are durable in Postgres before the caller is
	// acknowledged, without holding the global lock during DB I/O. Previously this
	// was a fire-and-forget goroutine, so a crash between the response and the async
	// write silently lost the transaction and its debit.
	var persist func()
	defer func() {
		if persist != nil {
			persist()
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.users[userID]; !ok {
		return TransactionResult{}, errors.New("user not found")
	}
	if idempotencyKey == "" {
		return TransactionResult{}, errors.New("idempotency key is required")
	}

	cacheKey := userID + ":" + idempotencyKey
	if cached, ok := s.idempotencyResult[cacheKey]; ok {
		// H-10: treat an expired entry as a miss so the TTL is honoured on read.
		if exp, has := s.idempotencyExpiry[cacheKey]; !has || time.Now().UTC().Before(exp) {
			return cached, nil
		}
		delete(s.idempotencyResult, cacheKey)
		delete(s.idempotencyExpiry, cacheKey)
	}

	if req.AmountPaise <= 0 {
		return TransactionResult{}, errors.New("amount must be greater than zero")
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return TransactionResult{}, errors.New("recipient is required")
	}

	if s.freezeState[userID] {
		reason := "Outgoing transactions are currently frozen for your protection."
		res := TransactionResult{
			TransactionID:  s.nextID("txn"),
			Status:         "blocked",
			RiskLevel:      transaction.RiskHigh,
			RiskScore:      100,
			XAIReason:      reason,
			StepUpRequired: true,
		}
		s.setIdempotentLocked(cacheKey, res)
		s.appendAuditLocked(userID, "transaction_blocked", "transaction", "blocked", reason, "AI")
		_, _ = s.ledger.WriteEvent(context.Background(), userID, "transaction_blocked", "transaction", res)
		return res, nil
	}

	now := time.Now().UTC()
	history := s.transactions[userID]
	// Floor the denominator at FLOOR_AVG (spec §1.1): a new account or a
	// tiny-average user must not divide by ~0 and produce an unbounded ratio.
	avgAmount := historicalAverageAmount(history)
	if floor := float64(s.params.AmountTerm.FloorAvgPaise); avgAmount < floor {
		avgAmount = floor
	}

	_, seenRecipient := s.beneficiaries[userID][strings.ToLower(req.Recipient)]
	// Extended raw inputs for the 32-feature transaction_risk_model (unit-safe
	// subset the backend already holds; unset fields mean-default at inference).
	var acctBalanceRupees, acctAgeDays float64
	var acctType string
	if accts := s.accounts[userID]; len(accts) > 0 {
		acctBalanceRupees = float64(accts[0].BalancePaise) / 100
		acctType = accts[0].AccountType
	}
	if u := s.users[userID]; u != nil {
		acctAgeDays = now.Sub(u.CreatedAt).Hours() / 24
	}
	signal := transaction.RiskSignal{
		AmountVsAverage:   float64(req.AmountPaise) / avgAmount,
		IsNewRecipient:    !seenRecipient,
		HourOfDay:         now.Hour(),
		FailedPINAttempts: req.FailedPINAttempts,
		VelocityCount1Hr:  velocityLastHour(history, now),
		BalanceImpact:     balanceImpact(history, req.AmountPaise, s.accounts[userID]),
		RecipientGNNScore: s.fraudGraph.GetRiskScore(req.Recipient),
		BehaviourDrift:    clamp(req.BehaviourDrift, 0, 1),
		SessionTrustScore: clamp(req.SessionTrustScore, 0, 1),

		TransactionAmount: float64(req.AmountPaise) / 100,
		CurrentBalance:    acctBalanceRupees,
		AccountAgeDays:    int(acctAgeDays),
		AccountType:       acctType,
		PaymentChannel:    req.Channel,
		Currency:          "INR",
	}

	assessment := s.riskEngine.Evaluate(signal)
	status := "success"
	stepUp := false
	var cooling *time.Time

	// Adaptive auth: escalate using 5-tier system per spec §3A.3 Layer 4
	authDecision := s.adaptiveAuth.EvaluateAuthTier(
		security.SessionRiskProfile{
			UserID:             userID,
			DeviceTrustScore:   clamp(req.SessionTrustScore, 0, 1),
			BehaviouralDrift:   clamp(req.BehaviourDrift, 0, 1),
			FailedAuthAttempts: req.FailedPINAttempts,
			IsTrustedDevice:    s.users[userID] != nil && s.users[userID].DeviceBound,
		},
		security.TransactionRiskProfile{
			RiskScore:       assessment.Score,
			RiskLevel:       string(assessment.Level),
			AmountPaise:     req.AmountPaise,
			IsNewRecipient:  !seenRecipient,
			AmountVsAverage: float64(req.AmountPaise) / avgAmount,
		},
	)
	if authDecision.IsBlocked {
		status = "blocked"
		stepUp = true
		// Use the configured window (spec §6.3 base = 240 min) instead of a
		// hardcoded 6h. CoolingOffParams.BaseMinutes is validated at startup and
		// overridable with FINIX_COOLOFF_BASE_MINUTES — previously that setting
		// existed but nothing read it, so the lock was always 6h and a demo
		// could not recover after showing a blocked transaction.
		t := now.Add(s.coolingOffWindow())
		cooling = &t
	} else if authDecision.RequireOTP || authDecision.RequireAck {
		status = "warning_ack_required"
		stepUp = true
	}

	// Generate txn ID early for outbox event
	txnID := s.nextID("txn")

	// Publish risk.decision to outbox for downstream consumers (Python RAG validation, monitoring)
	if s.outbox != nil {
		riskEvent := map[string]interface{}{
			"tx_id":         txnID,
			"user_id":       userID,
			"score":         assessment.Score,
			"level":         string(assessment.Level),
			"status":        status,
			"model_version": s.params.Version,
			"ts":            now.UTC().Format(time.RFC3339Nano),
		}
		if payload, err := json.Marshal(riskEvent); err == nil {
			s.outbox.Enqueue("risk.decision", string(payload))
		}
	}

	txn := Transaction{
		ID:               txnID,
		UserID:           userID,
		AmountPaise:      req.AmountPaise,
		Currency:         "INR",
		DebitCredit:      "debit",
		Recipient:        req.Recipient,
		MerchantName:     req.Recipient,
		Channel:          firstNonEmpty(req.Channel, "upi"),
		Description:      fmt.Sprintf("Payment to %s of INR %.2f", req.Recipient, float64(req.AmountPaise)/100),
		Status:           status,
		AssignedCategory: "",
		RiskLevel:        assessment.Level,
		RiskScore:        assessment.Score,
		XAIReason:        authDecision.XAIReason,
		LinkedAccount:    "",
		PaymentIntentID:  "",
		UPIIntentID:      "",
		BankReference:    "",
		TimelineIDs:      []string{},
		CreatedAt:        now,
		IdempotencyKey:   idempotencyKey,
		CoolingOffUntil:  cooling,
	}

	// Sign the transaction with Dilithium for post-quantum non-repudiation
	var dilithiumSig *security.DilithiumSignature
	if s.dilithium != nil {
		txData := []byte(fmt.Sprintf("%s:%s:%d:%s:%s", txn.ID, userID, txn.AmountPaise, txn.Recipient, now.Format(time.RFC3339Nano)))
		sig, signErr := s.dilithium.Sign(txData)
		if signErr == nil {
			dilithiumSig = sig
			txn.DilithiumSignature = sig
		} else {
			slog.Warn("failed to sign transaction with dilithium", "txId", txn.ID, "error", signErr)
		}
	}

	s.transactions[userID] = append(s.transactions[userID], txn)
	if status != "blocked" {
		s.beneficiaries[userID][strings.ToLower(req.Recipient)] = struct{}{}
	}

	// Feed the fraud graph so GNN risk propagation stays current
	s.fraudGraph.AddTransaction(userID, req.Recipient, req.AmountPaise)
	s.rebuildMuleScores()

	// H-8 fix: enforce the blockchain smart-contract rules BEFORE money moves.
	// Previously the event was written with WriteEvent, which only annotated a
	// violation (double-spend / active cooling-off) and still committed — an
	// application-layer bypass meant the blockchain layer did nothing. The strict
	// write rejects a violating transaction here, before the debit, so the
	// cooling-off and double-spend guarantees are real. The event is still
	// recorded on the append-only ledger for audit either way.
	if _, err := s.ledger.WriteEventStrict(context.Background(), userID, "transaction_initiated", "transaction", txn); err != nil {
		if errors.Is(err, blockchain.ErrSmartContractViolation) {
			// Flip the just-appended transaction to blocked and reject without debiting.
			if txns := s.transactions[userID]; len(txns) > 0 {
				txns[len(txns)-1].Status = "blocked"
			}
			s.appendAuditLocked(userID, "transaction_initiated", "transaction", "blocked",
				"Blocked by blockchain smart contract: "+err.Error(), "SmartContract")
			return TransactionResult{}, err
		}
		return TransactionResult{}, err
	}

	if status == "success" {
		// Double-entry accounting: debit user's account, credit external recipient
		acct := s.accounts[userID]
		if len(acct) > 0 {
			narration := fmt.Sprintf("Payment to %s — %s", req.Recipient, FormatMoney(req.AmountPaise))
			s.accountingLedger.PostOutgoingPayment(txn.ID, acct[0].ID, userID, req.Recipient, req.AmountPaise, narration)
		}
		if err := s.applyDebitToFirstAccountLocked(userID, req.AmountPaise); err != nil {
			return TransactionResult{}, err
		}
	}

	// Trigger threat response playbook for confirmed high-risk blocks
	if status == "blocked" && assessment.Score >= 70 {
		resp := s.threatResponse.RespondToThreat(security.ThreatDeviceCompromise, userID, map[string]any{
			"txId":      txn.ID,
			"recipient": req.Recipient,
			"riskScore": assessment.Score,
		})
		go s.PublishSSE(userID, "threat_response", resp)
	}

	eventOutcome := status
	s.appendAuditLocked(userID, "transaction_initiated", "transaction", eventOutcome, fmt.Sprintf("Transaction %s for INR %.2f", status, float64(req.AmountPaise)/100), "AI")

	res := TransactionResult{
		TransactionID:       txn.ID,
		Status:              txn.Status,
		RiskLevel:           txn.RiskLevel,
		RiskScore:           txn.RiskScore,
		XAIReason:           txn.XAIReason,
		CoolingOffUntil:     txn.CoolingOffUntil,
		StepUpRequired:      stepUp,
		RequiredTier:        authDecision.TierName,
		RequireBiometric:    authDecision.RequireBiometric,
		RequireOTP:          authDecision.RequireOTP,
		RequireManualReview: authDecision.RequireManualReview,
		DilithiumSignature:  dilithiumSig,
	}

	s.setIdempotentLocked(cacheKey, res)
	s.PushTransactionEvent(userID, txn)
	if status == "blocked" {
		s.PushNotification(userID, "security", "Transaction Blocked", fmt.Sprintf("INR %.2f to %s was blocked due to risk assessment.", float64(req.AmountPaise)/100, req.Recipient))
	}
	if s.db != nil {
		savedTxn := txn
		dbh := s.db
		uid := userID
		// Only snapshot account balances to persist when money actually moved.
		var accts []Account
		if status == "success" {
			accts = append([]Account(nil), s.accounts[userID]...)
		}
		persist = func() {
			// Runs after the lock is released (see the M-5 note at the top). The
			// circuit breaker bounds a slow/unavailable DB.
			s.saveWithCircuitBreaker("saveTransaction", uid, func() error {
				return dbh.SaveTransactionToDB(context.Background(), uid, savedTxn)
			})
			for _, a := range accts {
				acct := a
				s.saveWithCircuitBreaker("saveAccount", uid, func() error {
					return dbh.SaveAccountToDB(context.Background(), acct, uid)
				})
			}
		}
	}
	return res, nil
}

func (s *Service) saveWithCircuitBreaker(operationName, userID string, fn func() error) {
	err := s.dbCircuitBreaker.Execute(context.Background(), fn)
	if err != nil {
		s.outbox.Enqueue(operationName, userID)
		slog.Warn("outbox enqueued failed operation", "operation", operationName, "userId", userID, "error", err)
	}
}

func (s *Service) OverrideTransaction(userID string, req OverrideRequest) (TransactionResult, error) {
	if strings.TrimSpace(req.TransactionID) == "" {
		return TransactionResult{}, errors.New("transaction id is required")
	}
	if !req.BiometricOK || strings.TrimSpace(req.OTP) == "" {
		return TransactionResult{}, errors.New("biometric and otp are required")
	}
	// C-3 fix: actually verify the OTP against the stored value (was only checked
	// for non-emptiness). Done BEFORE acquiring s.mu, since VerifyOTPToken takes
	// s.mu itself and the RWMutex is not reentrant.
	if err := s.VerifyOTPToken(userID, req.OTP); err != nil {
		return TransactionResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	history := s.transactions[userID]
	for i := range history {
		tx := &history[i]
		if tx.ID != req.TransactionID {
			continue
		}
		if tx.Status != "blocked" && tx.Status != "warning_ack_required" {
			return TransactionResult{}, errors.New("transaction is not pending override")
		}

		// High-value overrides (>= ₹1,00,000) are flagged for manual review even
		// after biometric + OTP, per spec §3A.3 / §6 (Tier-3 manual-review queue).
		const highValuePaise = 100000 * 100 // ₹1,00,000
		manualReview := tx.AmountPaise >= highValuePaise

		tx.Status = "success"
		if err := s.applyDebitToFirstAccountLocked(userID, tx.AmountPaise); err != nil {
			return TransactionResult{}, err
		}
		s.beneficiaries[userID][strings.ToLower(tx.Recipient)] = struct{}{}
		s.transactions[userID] = history

		s.appendAuditLocked(userID, "transaction_override", "transaction", "success", "High-risk transaction overridden after step-up authentication", "User")
		_, _ = s.ledger.WriteEvent(context.Background(), userID, "transaction_override", "transaction", tx)

		if manualReview {
			s.appendAuditLocked(userID, "manual_review_queued", "transaction", "flagged",
				fmt.Sprintf("High-value override (%s) queued for manual review", FormatMoney(tx.AmountPaise)), "System")
			_, _ = s.ledger.WriteEvent(context.Background(), userID, "manual_review_queued", "audit", map[string]any{
				"transactionId": tx.ID, "amountPaise": tx.AmountPaise, "reason": "high_value_override",
			})
		}

		return TransactionResult{
			TransactionID:       tx.ID,
			Status:              tx.Status,
			RiskLevel:           tx.RiskLevel,
			RiskScore:           tx.RiskScore,
			XAIReason:           "Override completed after biometric + OTP verification.",
			StepUpRequired:      false,
			RequireManualReview: manualReview,
		}, nil
	}

	return TransactionResult{}, errors.New("transaction not found")
}

func (s *Service) ValidateRisk(ctx context.Context, txID string, req ValidateRiskRequest) (ValidateRiskResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the transaction
	var tx *Transaction
	for _, userTxs := range s.transactions {
		for i := range userTxs {
			if userTxs[i].ID == txID {
				tx = &userTxs[i]
				break
			}
		}
		if tx != nil {
			break
		}
	}
	if tx == nil {
		return ValidateRiskResponse{}, errors.New("transaction not found")
	}

	// Check if within cooling-off window
	now := time.Now().UTC()
	if tx.CoolingOffUntil != nil && now.After(*tx.CoolingOffUntil) {
		return ValidateRiskResponse{Applied: false, NewStatus: tx.Status}, errors.New("cooling-off window expired")
	}

	// Apply validation decision
	oldStatus := tx.Status
	applied := false
	newStatus := tx.Status

	if req.Verdict.AgreesWithML {
		// ML decision stands - no action needed unless escalation
		if req.Verdict.SuggestedLevel == "critical" && tx.Status != "blocked" {
			tx.Status = "blocked"
			newStatus = "blocked"
			applied = true
		}
	} else {
		// Disagrees with ML - apply recommended action
		switch req.Verdict.RecommendedAction {
		case "step_up", "cooling_off":
			if tx.Status != "blocked" {
				tx.Status = "warning_ack_required"
				newStatus = "warning_ack_required"
				applied = true
			}
		case "block":
			tx.Status = "blocked"
			newStatus = "blocked"
			applied = true
		case "dismiss":
			// Dismiss the risk - keep as success
			if tx.Status == "blocked" || tx.Status == "warning_ack_required" {
				tx.Status = "success"
				newStatus = "success"
				applied = true
			}
		}
	}

	// Update risk level if suggested
	if req.Verdict.SuggestedLevel != "" {
		switch req.Verdict.SuggestedLevel {
		case "low":
			tx.RiskLevel = transaction.RiskLow
		case "medium":
			tx.RiskLevel = transaction.RiskMedium
		case "high":
			tx.RiskLevel = transaction.RiskHigh
		}
	}

	// Persist validation to audit/ledger
	s.appendAuditLocked(tx.UserID, "risk_validation", "transaction", "applied",
		fmt.Sprintf("Risk validation applied: agrees_with_ml=%v, action=%s, new_status=%s",
			req.Verdict.AgreesWithML, req.Verdict.RecommendedAction, newStatus), "AI")
	_, _ = s.ledger.WriteEvent(context.Background(), tx.UserID, "risk_validation", "transaction", map[string]any{
		"tx_id":              txID,
		"agrees_with_ml":     req.Verdict.AgreesWithML,
		"suggested_level":    req.Verdict.SuggestedLevel,
		"confidence":         req.Verdict.Confidence,
		"rationale":          req.Verdict.Rationale,
		"recommended_action": req.Verdict.RecommendedAction,
		"old_status":         oldStatus,
		"new_status":         newStatus,
	})

	return ValidateRiskResponse{
		Applied:   applied,
		NewStatus: newStatus,
		OldStatus: oldStatus,
	}, nil
}

// ValidateRiskRequest is the request payload for risk validation
type ValidateRiskRequest struct {
	TxID    string            `json:"tx_id"`
	Verdict ValidationVerdict `json:"verdict"`
}

// ValidateRiskResponse is the response from risk validation
type ValidateRiskResponse struct {
	Applied   bool   `json:"applied"`
	NewStatus string `json:"new_status"`
	OldStatus string `json:"old_status"`
}

// ValidationVerdict is the verdict sent from Python validation worker
type ValidationVerdict struct {
	AgreesWithML      bool     `json:"agrees_with_ml"`
	SuggestedLevel    string   `json:"suggested_level"` // "low", "medium", "high", "critical"
	Confidence        float64  `json:"confidence"`
	Rationale         string   `json:"rationale"`
	RecommendedAction string   `json:"recommended_action"` // "allow", "step_up", "cooling_off", "block", "dismiss"
	Citations         []string `json:"citations"`
	XAIExplanation    string   `json:"xai_explanation"`
}

func (s *Service) TransactionHistory(userID string) []Transaction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	history := append([]Transaction(nil), s.transactions[userID]...)
	sort.Slice(history, func(i, j int) bool {
		return history[i].CreatedAt.After(history[j].CreatedAt)
	})
	return history
}

func (s *Service) CreateGoal(userID string, req GoalCreateRequest) (Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.TargetAmountPaise <= 0 {
		return Goal{}, errors.New("target amount must be greater than zero")
	}
	if strings.TrimSpace(req.Name) == "" {
		return Goal{}, errors.New("goal name is required")
	}
	if req.TargetDate.Before(time.Now()) {
		return Goal{}, errors.New("target date must be in the future")
	}

	now := time.Now().UTC()
	startDate := req.StartDate
	if startDate.IsZero() {
		startDate = now
	}
	goal := Goal{
		ID:                       s.nextID("goal"),
		Name:                     req.Name,
		Description:              req.Description,
		TargetAmountPaise:        req.TargetAmountPaise,
		Currency:                 firstNonEmpty(req.Currency, "INR"),
		SavedAmountPaise:         0,
		MonthlyContributionPaise: req.MonthlyContributionPaise,
		Frequency:                firstNonEmpty(req.Frequency, "monthly"),
		Priority:                 firstNonEmpty(req.Priority, "medium"),
		StartDate:                startDate,
		TargetDate:               req.TargetDate.UTC(),
		Status:                   "active",
		LinkedAccount:            req.LinkedAccount,
		CreatedAt:                now,
	}
	computeGoalProgress(&goal)
	s.goals[userID] = append(s.goals[userID], goal)
	s.appendAuditLocked(userID, "goal_created", "goal", "success", "Financial goal created", "User")
	_, _ = s.ledger.WriteEvent(context.Background(), userID, "goal_created", "goal", goal)
	if s.db != nil {
		savedGoal := goal
		db := s.db
		go func() {
			_ = db.SaveGoalToDB(context.Background(), savedGoal, userID)
		}()
	}
	return goal, nil
}

func computeGoalProgress(g *Goal) {
	if g.TargetAmountPaise > 0 {
		g.ProgressPercent = clamp(float64(g.SavedAmountPaise)/float64(g.TargetAmountPaise)*100, 0, 100)
	}
	shortfall := g.TargetAmountPaise - g.SavedAmountPaise
	if shortfall < 0 {
		shortfall = 0
	}
	g.ShortfallEstimatePaise = shortfall
}

func (s *Service) Goals(userID string) []Goal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	goals := append([]Goal(nil), s.goals[userID]...)
	for i := range goals {
		computeGoalProgress(&goals[i])
	}
	return goals
}

func (s *Service) GoalByID(userID, goalID string) (Goal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.goals[userID] {
		if g.ID == goalID {
			result := g
			computeGoalProgress(&result)
			return result, nil
		}
	}
	return Goal{}, errors.New("goal not found")
}

func (s *Service) ContributeGoal(userID, goalID string, req GoalContributionRequest) (Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// L-9: if an idempotency key is supplied, a repeat returns the first result
	// rather than debiting again (a double-tap / network retry is now safe).
	idemKey := ""
	if k := strings.TrimSpace(req.IdempotencyKey); k != "" {
		idemKey = userID + ":" + goalID + ":" + k
		if cached, ok := s.goalIdempotency[idemKey]; ok {
			return cached, nil
		}
	}
	if req.AmountPaise <= 0 {
		return Goal{}, errors.New("contribution must be greater than zero")
	}
	// M-14 fix: bound the contribution to prevent int64 overflow of SavedAmountPaise
	// (which could wrap negative and corrupt the completion check). Ceiling is
	// ₹10,00,000 in paise, matching the API-layer payment limit.
	const maxContributionPaise int64 = 100_000_000
	if req.AmountPaise > maxContributionPaise {
		return Goal{}, errors.New("contribution exceeds maximum allowed amount")
	}
	for i := range s.goals[userID] {
		if s.goals[userID][i].ID == goalID {
			if s.goals[userID][i].Status != "active" {
				return Goal{}, errors.New("goal is not active")
			}
			// M-14/correctness: overflow guard against the running total, and debit
			// BEFORE mutating goal state so a failed debit can't corrupt the goal.
			if s.goals[userID][i].SavedAmountPaise > math.MaxInt64-req.AmountPaise {
				return Goal{}, errors.New("contribution would overflow goal balance")
			}
			if err := s.applyDebitToFirstAccountLocked(userID, req.AmountPaise); err != nil {
				return Goal{}, err
			}
			s.goals[userID][i].SavedAmountPaise += req.AmountPaise
			if s.goals[userID][i].SavedAmountPaise >= s.goals[userID][i].TargetAmountPaise {
				s.goals[userID][i].Status = "completed"
			}
			goal := s.goals[userID][i]
			computeGoalProgress(&goal)
			if idemKey != "" {
				s.goalIdempotency[idemKey] = goal
			}
			s.appendAuditLocked(userID, "goal_contribution", "goal", "success", "Goal contribution recorded", "User")
			_, _ = s.ledger.WriteEvent(context.Background(), userID, "goal_contribution", "goal", goal)
			return goal, nil
		}
	}
	return Goal{}, errors.New("goal not found")
}

func (s *Service) CloseGoal(userID, goalID string) (Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.goals[userID] {
		if s.goals[userID][i].ID == goalID {
			s.goals[userID][i].Status = "closed"
			goal := s.goals[userID][i]
			s.appendAuditLocked(userID, "goal_closed", "goal", "success", "Goal closed and funds unlocked", "User")
			_, _ = s.ledger.WriteEvent(context.Background(), userID, "goal_closed", "goal", goal)
			return goal, nil
		}
	}
	return Goal{}, errors.New("goal not found")
}

func (s *Service) HealthScore(userID string) (healthscore.Result, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return healthscore.Result{}, errors.New("user not found")
	}
	result := s.healthScoreLocked(userID)
	return result, nil
}

func (s *Service) PortfolioSummary(userID string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	assets := s.totalAssetPaiseLocked(userID)
	liability := s.totalLiabilityPaiseLocked(userID)
	netWorth := assets - liability
	ratio := 0.0
	if liability > 0 {
		ratio = float64(assets) / float64(liability)
	}
	return map[string]any{
		"assetsPaise":         assets,
		"liabilityPaise":      liability,
		"netWorthPaise":       netWorth,
		"assetLiabilityRatio": ratio,
		"healthScore":         s.healthScoreLocked(userID),
	}, nil
}

func (s *Service) Investments(userID string) ([]InvestmentHolding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	return append([]InvestmentHolding(nil), s.investments[userID]...), nil
}

func (s *Service) Insurance(userID string) ([]InsurancePolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	return append([]InsurancePolicy(nil), s.insurance[userID]...), nil
}

func (s *Service) Loans(userID string) ([]LoanRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	return append([]LoanRecord(nil), s.loans[userID]...), nil
}

func (s *Service) RunSimulation(userID string, req SimulationRequest) (SimulationResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return SimulationResult{}, errors.New("user not found")
	}
	if req.Years <= 0 {
		req.Years = 5
	}
	if req.Years > 30 {
		req.Years = 30
	}
	if req.Iterations <= 0 {
		req.Iterations = 1000
	}
	if req.Iterations > 10000 {
		req.Iterations = 10000
	}

	base := s.netWorthPaiseLocked(userID)
	adjustment := req.AmountPaise
	if adjustment <= 0 {
		adjustment = 100000
	}

	meanGrowth := 0.07
	stdDev := 0.12
	// Monte Carlo simulation with stochastic annual returns
	rng := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	results := make([]int64, req.Iterations)
	principal := float64(base + adjustment)
	for i := 0; i < req.Iterations; i++ {
		// Sample annual return from normal distribution via Box-Muller
		u1 := rng.Float64()
		u2 := rng.Float64()
		z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
		annualReturn := meanGrowth + stdDev*z
		if annualReturn < -0.3 {
			annualReturn = -0.3
		}
		// Compound growth: (1+r)^years
		projected := int64(principal * math.Pow(1.0+annualReturn, float64(req.Years)))
		if projected < 0 {
			projected = 0
		}
		results[i] = projected
	}
	sort.Slice(results, func(i, j int) bool { return results[i] < results[j] })

	p10 := results[req.Iterations*10/100]
	p25 := results[req.Iterations*25/100]
	p50 := results[req.Iterations*50/100]
	p75 := results[req.Iterations*75/100]
	p90 := results[req.Iterations*90/100]

	future := int64(principal * math.Pow(1.0+meanGrowth, float64(req.Years)))
	healthDelta := 8
	if strings.Contains(strings.ToLower(req.Scenario), "loan") {
		healthDelta = 12
	}

	return SimulationResult{
		Scenario:                req.Scenario,
		ProjectedNetWorthNow:    base,
		ProjectedNetWorthFuture: future,
		GoalCompletionDeltaPct:  6.4,
		HealthScoreDelta:        healthDelta,
		MonteCarloPercentiles:   []int64{p10, p25, p50, p75, p90},
		ConfidenceInterval90:    []int64{p10, p90},
		XAIReason:               fmt.Sprintf("Monte Carlo simulation over %d iterations projects net worth growth from %s to median %s with 90%% CI [%s – %s].", req.Iterations, FormatMoney(base), FormatMoney(p50), FormatMoney(p10), FormatMoney(p90)),
	}, nil
}

func (s *Service) InsightsFeed(userID string) ([]InsightCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}

	netWorth := s.netWorthPaiseLocked(userID)
	persona := s.detectPersonaLocked(userID)
	cards := []InsightCard{
		{ID: s.nextID("insight"), Title: "Payday Goal Nudge", Body: "Allocating INR 4,200 toward your emergency fund keeps you on track.", Reason: "Based on your cashflow trend and active goals", Action: "Contribute now", PersonaTag: persona, Priority: "high", Snoozeable: true, RelatedGoalID: "", ActionLinks: []string{"/goals/contribute", "/simulation"}},
		{ID: s.nextID("insight"), Title: "Portfolio Balance", Body: "Your equity concentration is slightly high. Consider adding debt instruments.", Reason: "Diversification score dropped below your target", Action: "Run simulation", PersonaTag: persona, Priority: "medium", Snoozeable: true, RelatedGoalID: "", ActionLinks: []string{"/simulation", "/portfolio/rebalance"}},
		{ID: s.nextID("insight"), Title: "Security Tip", Body: "Never share OTP, UPI PIN, or CVV over calls or messages.", Reason: "High fraud campaign frequency in your region", Action: "View security corner", PersonaTag: "all", Priority: "low", Snoozeable: false, RelatedGoalID: "", ActionLinks: []string{"/security-corner"}},
	}

	if netWorth < 5000000 {
		cards = append(cards, InsightCard{ID: s.nextID("insight"), Title: "Emergency Fund Reminder", Body: "Your liquid buffer is below 6 months. Top up for resilience.", Reason: "Liquidity pillar below target", Action: "Review health score", PersonaTag: persona, Priority: "high", Snoozeable: true, RelatedGoalID: "", ActionLinks: []string{"/health-score"}})
	}
	return cards, nil
}

func (s *Service) MarketSnapshot(userID string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}

	return map[string]any{
		"sensex":               75180.42,
		"nifty":                22831.11,
		"goldPer10g":           74250.00,
		"repoRate":             6.50,
		"portfolioImpactPaise": int64(12450),
		"xai":                  "Rate-sensitive holdings may benefit from stable policy rates while debt instruments remain attractive.",
	}, nil
}

func (s *Service) Persona(userID string) (PersonaResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return PersonaResponse{}, errors.New("user not found")
	}

	profile := s.detectPersonaLocked(userID)
	explanation := "You consistently preserve liquidity and prefer lower-volatility instruments."
	traits := []string{"low_risk_tolerance", "consistent_saver", "liquidity_focused"}
	confidence := 0.78
	if len(s.transactions[userID]) > 20 {
		profile = "Life-Event Driven"
		explanation = "Your transaction and goal activity clusters around milestone spending windows."
		traits = []string{"milestone_focused", "moderate_risk_tolerance", "event_driven"}
		confidence = 0.82
	}
	if len(s.investments[userID]) > 5 {
		profile = "Active Investor"
		explanation = "Your holding diversity and order frequency indicate active portfolio management."
		traits = []string{"active_trader", "high_risk_tolerance", "diversified"}
		confidence = 0.85
	}

	return PersonaResponse{Persona: profile, Explanation: explanation, ContestRoute: "/v1/insights/persona/contest", Traits: traits, Confidence: confidence}, nil
}

func (s *Service) SecurityStatus(userID string) (SecurityHealth, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[userID]
	if !ok {
		return SecurityHealth{}, errors.New("user not found")
	}

	return SecurityHealth{
		BiometricLock:         user.BiometricEnabled,
		DeviceBinding:         user.DeviceBound,
		SIMBinding:            user.SIMBound,
		TransactionMonitoring: true,
		SMSScannerEnabled:     s.consents[userID]["sms_scanning"],
		AntiPhishingContacts:  true,
		AccountFrozen:         s.freezeState[userID],
	}, nil
}

func (s *Service) ScanSMS(userID string, req SMSScanRequest) (SMSScanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return SMSScanResult{}, errors.New("user not found")
	}
	if strings.TrimSpace(req.Sender) == "" || strings.TrimSpace(req.Message) == "" {
		return SMSScanResult{}, errors.New("sender and message are required")
	}

	// Verify hash-chain for integrity
	hcVerified := false
	if s.ledger != nil {
		ok, _ := s.ledger.VerifyIntegrity()
		hcVerified = ok
	}

	// Check device binding
	deviceID := ""
	trustedDevice := false
	deviceTrustScore := 0.0
	if u, ok := s.users[userID]; ok {
		deviceID = shortenFingerprint(u.ID + ":device")
		trustedDevice = u.DeviceBound
		deviceTrustScore = 0.85
		if !trustedDevice {
			deviceTrustScore = 0.25
		}
	}

	sender := strings.ToUpper(strings.TrimSpace(req.Sender))
	message := strings.ToLower(req.Message)
	badge := "green"
	reason := "Sender and template match known safe banking patterns."
	indicators := []string{}
	urlResult := "none"

	// Extract URLs from message
	urlPattern := regexp.MustCompile(`https?://[^\s]+`)
	urls := urlPattern.FindAllString(message, -1)
	if len(urls) > 0 {
		urlResult = "suspicious"
		for _, u := range urls {
			if strings.Contains(u, "bit.ly") || strings.Contains(u, "tinyurl") || strings.Contains(u, "shorturl") {
				indicators = append(indicators, "short_url_"+u)
				urlResult = "malicious"
			} else if strings.Contains(u, "bank") || strings.Contains(u, "upi") || strings.Contains(u, "sbi") || strings.Contains(u, "hdfc") {
				if !strings.Contains(u, strings.ToLower(sender)) {
					indicators = append(indicators, "spoofed_bank_url_"+u)
					urlResult = "malicious"
				}
			}
		}
	}

	if strings.Contains(message, "share otp") || strings.Contains(message, "share your otp") || strings.Contains(message, "otp share") {
		indicators = append(indicators, "otp_solicitation")
		badge = "red"
		reason = "Message requests OTP sharing — no legitimate bank asks for OTP sharing outside official flow."
	} else if strings.Contains(message, "upi pin") || strings.Contains(message, "internet banking pin") || strings.Contains(message, "net banking pin") {
		indicators = append(indicators, "pin_solicitation")
		badge = "red"
		reason = "Message requests banking PIN — this is a hallmark of credential theft."
	} else if strings.Contains(message, "immediately click") || strings.Contains(message, "click immediately") || strings.Contains(message, "action required") {
		indicators = append(indicators, "urgency_social_engineering")
		if badge == "green" {
			badge = "amber"
			reason = "Urgency language detected. Verify independently before clicking any links."
		}
	} else if strings.HasPrefix(sender, "+91") || (len(urls) > 0 && urlResult == "malicious") {
		indicators = append(indicators, "non_bank_sender_format")
		badge = "red"
		reason = "Bank impersonation signal detected from non-bank sender or malicious URL."
	} else if !strings.HasPrefix(sender, "VK-") && !strings.HasPrefix(sender, "VM-") {
		indicators = append(indicators, "unregistered_sender")
		if badge == "green" {
			badge = "amber"
			reason = "Sender not matched in RBI-style bank sender pattern. Use caution before acting."
		}
	}

	s.appendAuditLocked(userID, "sms_scanned", "security", badge, "SMS fraud classification: "+reason, "AI")
	_, _ = s.ledger.WriteEvent(context.Background(), userID, "sms_scanned", "security", map[string]any{"badge": badge, "reason": reason, "indicators": indicators})

	// If RAG client is configured, delegate to Python FINIX RAG service for enhanced analysis
	if s.ragClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		ragResp, err := s.ragClient.ScanSMS(ctx, req.Sender, req.Message)
		if err == nil {
			// Use RAG response if confidence is high
			if ragResp.Confidence >= 0.85 {
				badge = ragResp.Badge
				reason = ragResp.Reason
				indicators = ragResp.Indicators
				// Use first indicator as URL scan result indicator if available
				if len(ragResp.Indicators) > 0 {
					urlResult = ragResp.Indicators[0]
				}
			}
		} else {
			slog.Warn("RAG SMS scan failed, using local classification", "error", err)
		}
	}

	return SMSScanResult{
		Badge:              badge,
		Reason:             reason,
		DeviceTrustScore:   deviceTrustScore,
		RegisteredDeviceID: deviceID,
		TrustedDevice:      trustedDevice,
		PhishingIndicators: indicators,
		URLScanResult:      urlResult,
		HashChainVerified:  hcVerified,
	}, nil
}

func (s *Service) EmergencyFreeze(userID string, req FreezeRequest) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	if !req.TestMode {
		s.freezeState[userID] = true
	}
	mode := "live"
	if req.TestMode {
		mode = "test"
	}
	s.appendAuditLocked(userID, "emergency_freeze", "security", "success", "Emergency freeze flow triggered", "User")
	_, _ = s.ledger.WriteEvent(context.Background(), userID, "emergency_freeze", "security", map[string]any{"mode": mode})
	if s.db != nil {
		frozen := s.freezeState[userID]
		db := s.db
		go func() {
			_ = db.SaveFreezeStateToDB(context.Background(), userID, frozen)
		}()
	}
	return map[string]any{"frozen": s.freezeState[userID], "mode": mode, "message": "Your accounts are protected."}, nil
}

func (s *Service) Unfreeze(userID string, req UnfreezeRequest) (map[string]any, error) {
	if !req.biometricSatisfied() {
		return nil, errors.New("biometric verification is required")
	}

	// Step-up policy. Production (FINIX_REQUIRE_BIOMETRIC_CHALLENGE=true) keeps
	// the C-4 control: an OTP must be supplied AND verified against the stored
	// value before the freeze is lifted. With the flag off (demo posture) a
	// biometric-only unfreeze is accepted, matching the mobile client which has
	// no OTP step in its unfreeze flow.
	//
	// An OTP that IS supplied is always verified, in either mode, so a wrong
	// code never unfreezes the account.
	otp := strings.TrimSpace(req.OTP)
	if unfreezeOTPRequired() && otp == "" {
		return nil, errors.New("biometric and otp are required")
	}
	if otp != "" {
		// Done before s.mu.Lock — VerifyOTPToken locks s.mu.
		if err := s.VerifyOTPToken(userID, otp); err != nil {
			return nil, err
		}
	} else {
		slog.Warn("unfreeze accepted with biometric only; set FINIX_REQUIRE_BIOMETRIC_CHALLENGE=true to require OTP step-up",
			"user", userID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	s.freezeState[userID] = false
	s.appendAuditLocked(userID, "emergency_unfreeze", "security", "success", "Emergency freeze removed after step-up verification", "User")
	_, _ = s.ledger.WriteEvent(context.Background(), userID, "emergency_unfreeze", "security", map[string]bool{"frozen": false})
	return map[string]any{"frozen": false, "message": "Outgoing transactions are enabled again."}, nil
}

func (s *Service) ReportFraud(userID string, req FraudReportRequest) (FraudReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return FraudReport{}, errors.New("user not found")
	}
	if strings.TrimSpace(req.Description) == "" {
		return FraudReport{}, errors.New("description is required")
	}
	report := FraudReport{ID: s.nextID("fraud"), UserID: userID, Description: req.Description, CreatedAt: time.Now().UTC()}
	s.fraudReports = append(s.fraudReports, report)
	// Flag the reported entity in the fraud graph for future GNN propagation
	s.fraudGraph.MarkAsFraud(strings.TrimSpace(req.Description))
	s.appendAuditLocked(userID, "fraud_report_submitted", "security", "success", "Fraud report submitted", "User")
	_, _ = s.ledger.WriteEvent(context.Background(), userID, "fraud_report_submitted", "security", report)
	return report, nil
}

func (s *Service) TaxDashboard(userID string) (TaxDashboard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return TaxDashboard{}, errors.New("user not found")
	}

	profile, _ := s.kycProfiles[userID]
	taxableIncome := s.estimateTaxableIncomeLocked(userID)
	regime := "new"
	if profile != nil && profile.TaxResidency != "" {
		regime = profile.TaxResidency
	}
	var liability int64
	if regime == "old" {
		liability = calculateOldRegimeTax(taxableIncome)
	} else {
		liability = calculateNewRegimeTax(taxableIncome)
	}

	tds := taxableIncome * 10 / 1000
	if tds > liability {
		tds = liability
	}
	net := liability - tds
	if net < 0 {
		net = 0
	}
	deadline := time.Date(time.Now().Year(), 7, 31, 23, 59, 0, 0, time.Local)
	if time.Now().After(deadline) {
		deadline = deadline.AddDate(1, 0, 0)
	}

	pan := ""
	if profile != nil {
		pan = profile.PANMasked
	}

	return TaxDashboard{
		PANMasked:                  pan,
		FinancialYear:              fmt.Sprintf("FY%d-%d", time.Now().Year(), time.Now().Year()+1),
		TaxableIncomePaise:         taxableIncome,
		TaxRegime:                  regime,
		EstimatedTaxLiabilityPaise: liability,
		TdsDeductedPaise:           tds,
		NetPayablePaise:            net,
		ITRStatus:                  "not_filed",
		ITRReferenceNumber:         "",
		TaxFilingProvider:          "",
		Suggestions:                s.taxSuggestionsLocked(userID, regime, liability),
		DaysToITRDeadline:          int(time.Until(deadline).Hours() / 24),
		IncomeBreakdown: map[string]int64{
			"salary":        taxableIncome * 75 / 100,
			"capital_gains": taxableIncome * 15 / 100,
			"other_income":  taxableIncome * 10 / 100,
		},
		Deductions: s.estimateDeductionsLocked(userID),
	}, nil
}

func (s *Service) RegimeComparison(userID string) (TaxRegimeComparison, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return TaxRegimeComparison{}, errors.New("user not found")
	}
	taxableIncome := s.estimateTaxableIncomeLocked(userID)
	oldTax := calculateOldRegimeTax(taxableIncome)
	newTax := calculateNewRegimeTax(taxableIncome)
	recommended := "old"
	reason := "Higher deduction utilization under 80C, 80D, and home-loan interest makes old regime lower for your profile."
	if newTax < oldTax {
		recommended = "new"
		reason = "Lower deductions and salary composition make new regime currently more efficient."
	}
	return TaxRegimeComparison{OldRegimeTaxPaise: oldTax, NewRegimeTaxPaise: newTax, Recommended: recommended, XAIReason: reason}, nil
}

func (s *Service) Deductions(userID string) ([]Deduction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	return s.estimateDeductionsLocked(userID), nil
}

func (s *Service) CapitalGains(userID string) (CapitalGains, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return CapitalGains{}, errors.New("user not found")
	}
	var stcg, ltcg int64
	for _, inv := range s.investments[userID] {
		gain := inv.CurrentValuePaise - inv.InvestedPaise
		if gain <= 0 {
			continue
		}
		// If we have a tax lot date, check holding period
		holdingPeriod := time.Since(inv.TaxLotDate).Hours() / (24 * 365)
		if inv.TaxLotDate.IsZero() || holdingPeriod < 1 {
			stcg += gain
		} else {
			ltcg += gain
		}
	}
	tip := "Waiting for holding period completion can reduce tax incidence on select holdings."
	if stcg > ltcg {
		tip = "Consider holding short-term positions beyond 12 months to qualify for LTCG indexation benefits."
	}
	return CapitalGains{STCGPaise: stcg, LTCGPaise: ltcg, Tip: tip}, nil
}

func (s *Service) Chat(userID string, req ChatRequest) (ChatResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return ChatResponse{}, errors.New("user not found")
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return ChatResponse{}, errors.New("prompt is required")
	}

	disclaimer := "Disclaimer: This is AI-generated advice. Please consult a certified financial advisor before making financial decisions."
	now := time.Now().UTC()
	contextSnapshot := map[string]any{
		"user_id":   userID,
		"timestamp": now,
	}

	// If RAG client is configured, delegate to Python FINIX RAG service
	if s.ragClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Build chat history from recent conversations (last 10 messages)
		var history []ai.ChatMessage
		// TODO: fetch from chat history store if available

		ragReq := ai.ChatRequest{
			Query:      prompt,
			History:    history,
			UserID:     userID,
			DeviceFP:   "", // TODO: get from session
			Scope:      "general",
			MaxSources: 5,
		}

		ragResp, err := s.ragClient.Chat(ctx, ragReq)
		if err == nil && ragResp != nil {
			// Convert RAG response to platform ChatResponse
			reply := ragResp.Answer
			if !strings.Contains(strings.ToLower(reply), "disclaimer") {
				reply += "\n\n" + disclaimer
			}

			// Extract citation titles/IDs for explainability
			citationRefs := make([]string, len(ragResp.Citations))
			for i, c := range ragResp.Citations {
				if c.Title != "" {
					citationRefs[i] = c.Title
				} else if c.OKFID != "" {
					citationRefs[i] = c.OKFID
				} else {
					citationRefs[i] = "source"
				}
			}

			// Build suggestions from RAG response
			suggestions := []string{"Run a what-if simulation", "Check tax dashboard", "Scan an SMS"}
			if ragResp.ValidationSummary != nil {
				suggestions = append(suggestions, "Review risk validation details")
			}

			return ChatResponse{
				Reply:               reply,
				Disclaimer:          disclaimer,
				ContextSnapshot:     contextSnapshot,
				Suggestions:         suggestions,
				Explainability:      fmt.Sprintf("FINIX RAG response with %d citations, confidence: %.0f%%", len(ragResp.Citations), ragResp.Confidence*100),
				ModelID:             "finix-rag-llama3-70b",
				ModelVersion:        "1.0",
				LLMPromptSnapshot:   fmt.Sprintf("system: FINIX RAG\nuser: %s", prompt),
				ResponseGeneratedAt: now,
				RecommendationID:    s.nextID("rec"),
				ModelConfidence:     ragResp.Confidence,
			}, nil
		}
		// Fall through to local chatbot on RAG error
		slog.Warn("RAG chat failed, falling back to local chatbot", "error", err)
	}

	// Local chatbot fallback
	if s.chatbot != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		reply, err := s.chatbot.Query(ctx, defaultSystemPrompt, prompt, nil)
		if err == nil && strings.TrimSpace(reply) != "" {
			if !strings.Contains(strings.ToLower(reply), "disclaimer") {
				reply += "\n\n" + disclaimer
			}
			return ChatResponse{
				Reply:               reply,
				Disclaimer:          disclaimer,
				ContextSnapshot:     contextSnapshot,
				Suggestions:         []string{"Run a what-if simulation", "Check tax dashboard", "Scan an SMS"},
				Explainability:      "This response is generated by a large language model (Groq llama3-8b) with FINIX-specific system prompts.",
				ModelID:             "llama3-8b-8192",
				ModelVersion:        "1.0",
				LLMPromptSnapshot:   fmt.Sprintf("system: %s\nuser: %s", defaultSystemPrompt, prompt),
				ResponseGeneratedAt: now,
				RecommendationID:    s.nextID("rec"),
				ModelConfidence:     0.87,
			}, nil
		}
	}

	reply := s.templateReply(prompt)
	return ChatResponse{
		Reply:               reply,
		Disclaimer:          disclaimer,
		ContextSnapshot:     contextSnapshot,
		Suggestions:         []string{"Run a what-if simulation", "Check tax dashboard", "Scan an SMS"},
		Explainability:      "Template-based reply using intent classification on user prompt keywords.",
		ModelID:             "template-v1",
		ModelVersion:        "1.0",
		LLMPromptSnapshot:   fmt.Sprintf("user: %s", prompt),
		ResponseGeneratedAt: now,
		RecommendationID:    s.nextID("rec"),
		ModelConfidence:     0.65,
	}, nil
}

func (s *Service) templateReply(prompt string) string {
	reply := "I can help explain transaction alerts, run what-if simulations, and decode tax or portfolio changes in plain language."
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "blocked"):
		reply = "Your transaction may be blocked if it appears unusual by amount, recipient novelty, hour, or session-risk signals. You can review the XAI reason and use step-up verification if appropriate."
	case strings.Contains(lower, "tax"):
		reply = "Your regime comparison currently indicates the old regime has lower projected liability due to deduction utilization."
	case strings.Contains(lower, "sip") || strings.Contains(lower, "invest"):
		reply = "Increasing SIP generally improves long-term goal completion probability, but projected outcomes can vary with market volatility and contribution consistency."
	case strings.Contains(lower, "sms"):
		reply = "Paste suspicious SMS text and sender in Security Corner; the scanner checks sender pattern, URL risk, and social-engineering language."
	}
	reply = strings.ReplaceAll(reply, "guaranteed", "projected")
	return reply
}

func (s *Service) Profile(userID string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[userID]
	if !ok {
		return User{}, errors.New("user not found")
	}
	return *user, nil
}

func (s *Service) SetConsent(userID, consentType string, granted bool) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	consentType = strings.TrimSpace(consentType)
	if consentType == "" {
		return nil, errors.New("consent type is required")
	}

	if _, ok := s.consents[userID]; !ok {
		s.consents[userID] = make(map[string]bool)
	}
	s.consents[userID][consentType] = granted
	outcome := "revoked"
	if granted {
		outcome = "granted"
	}
	s.appendAuditLocked(userID, "consent_updated", "consent", outcome, "Consent preference changed", "User")
	if s.db != nil {
		savedType := consentType
		savedGranted := granted
		db := s.db
		go func() {
			_ = db.SaveConsentToDB(context.Background(), userID, savedType, savedGranted)
		}()
	}
	resp := map[string]any{"type": consentType, "granted": granted}
	if ledgerEvent, err := s.ledger.WriteEvent(context.Background(), userID, "consent_updated", "consent", map[string]any{"type": consentType, "granted": granted}); err == nil {
		resp["consent_id"] = ledgerEvent.ID
		resp["blockchain_tx_hash"] = ledgerEvent.Hash
	}
	return resp, nil
}

func (s *Service) SetNudgePreference(userID, preference string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return nil, errors.New("user not found")
	}
	preference = strings.ToLower(strings.TrimSpace(preference))
	if preference == "" {
		return nil, errors.New("nudge preference is required")
	}
	user.NudgePreference = preference
	s.appendAuditLocked(userID, "nudge_preference_updated", "settings", "success", "Nudge preference updated", "User")
	return map[string]any{"nudgePreference": preference}, nil
}

func (s *Service) NotificationSettings(userID string) ([]NotificationSetting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	settings := append([]NotificationSetting(nil), s.notifications[userID]...)
	return settings, nil
}

func (s *Service) SubmitFeedback(userID string, req FeedbackRequest) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	if strings.TrimSpace(req.Category) == "" || strings.TrimSpace(req.Message) == "" {
		return nil, errors.New("category and message are required")
	}
	s.appendAuditLocked(userID, "feedback_submitted", "settings", "success", "User submitted product feedback", "User")
	return map[string]any{"status": "received"}, nil
}

func (s *Service) AuditLogs(userID string) ([]AuditEvent, error) {
	// When the ledger is Fabric-backed, serve the audit log from the durable
	// event ledger (Postgres mirror, Fabric as backstop) instead of the volatile
	// in-memory s.audit slice. This is what makes the audit trail survive a
	// backend restart — the events live on-chain and in Postgres, not in RAM.
	if s.LedgerIsFabricBacked() {
		if events := s.ledger.EventsByUser(userID); len(events) > 0 {
			out := make([]AuditEvent, 0, len(events))
			for _, ev := range events {
				out = append(out, ledgerEventToAudit(ev))
			}
			sort.Slice(out, func(i, j int) bool {
				return out[i].Timestamp.After(out[j].Timestamp)
			})
			return out, nil
		}
		// No ledger rows yet (or a read hiccup) — fall through to the local view.
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	out := make([]AuditEvent, 0, len(s.audit))
	for _, item := range s.audit {
		if item.UserID == userID {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out, nil
}

// ledgerEventToAudit maps a durable ledger event onto the AuditEvent shape the
// audit API returns. Outcome comes from the smart-contract result; a rule
// violation surfaces as a failed outcome with the reason as its XAI detail.
func ledgerEventToAudit(ev blockchain.Event) AuditEvent {
	outcome := "success"
	xai := ""
	if ev.SmartContract.RuleName != "" {
		if ev.SmartContract.Passed {
			xai = "smart contract passed: " + ev.SmartContract.RuleName
		} else {
			outcome = "blocked"
			xai = ev.SmartContract.Violation
		}
	}
	return AuditEvent{
		ID:          ev.ID,
		Timestamp:   ev.Timestamp,
		UserID:      ev.UserID,
		EventType:   ev.Action,
		TriggeredBy: "ledger",
		Outcome:     outcome,
		Details:     ev.Resource,
		XAIReason:   xai,
	}
}

func (s *Service) AuditLogIntegrity(userID string) (AuditIntegrity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return AuditIntegrity{}, errors.New("user not found")
	}
	return AuditIntegrity{MerkleRoot: s.ledger.MerkleRoot(userID), EventCount: len(s.ledger.EventsByUser(userID))}, nil
}

func (s *Service) AuthProfile(userID string) (AuthProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[userID]
	if !ok {
		return AuthProfile{}, errors.New("user not found")
	}
	profile, ok := s.authProfiles[userID]
	if !ok {
		return AuthProfile{}, errors.New("auth profile not found")
	}
	result := *profile
	if result.InternalUserID == "" {
		result.InternalUserID = userID
	}
	result.KYCVerified = user.EKYCVerified
	result.BiometricEnabled = user.BiometricEnabled
	result.MobileNumber = DecryptField(result.MobileNumber)
	result.Email = DecryptField(result.Email)
	// M-11 fix: strip secret material in the service layer, not only in the handler.
	// An internal caller must never receive the PIN hash or a live OTP code.
	result.PasswordHash = ""
	result.OTPCode = ""
	return result, nil
}

func (s *Service) UpdateAuthProfile(userID string, req UpdateAuthProfileRequest) (AuthProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return AuthProfile{}, errors.New("user not found")
	}
	profile, ok := s.authProfiles[userID]
	if !ok {
		profile = &AuthProfile{
			InternalUserID:  userID,
			FullName:        user.Name,
			MobileNumber:    user.Mobile,
			Email:           user.Email,
			ChallengeStatus: "not_created",
		}
		s.authProfiles[userID] = profile
	}

	if value := strings.TrimSpace(req.FullName); value != "" {
		profile.FullName = value
		user.Name = value
	}
	if value := strings.TrimSpace(req.Email); value != "" {
		profile.Email = value
		user.Email = value
	}
	if value := strings.TrimSpace(req.DeviceIDFingerprint); value != "" {
		profile.DeviceIDFingerprint = value
	}
	if value := strings.TrimSpace(req.DeviceType); value != "" {
		profile.DeviceType = value
	}
	if value := strings.TrimSpace(req.AppVersion); value != "" {
		profile.AppVersion = value
	}
	if value := strings.TrimSpace(req.IPAddress); value != "" {
		profile.IPAddress = value
	}
	if value := strings.TrimSpace(req.DeviceChallengePublicKeyID); value != "" {
		profile.DeviceChallengePublicKeyID = value
	}
	if value := strings.TrimSpace(req.RegisteredKeyFingerprint); value != "" {
		profile.RegisteredKeyFingerprint = value
	}
	if req.TrustedDeviceFlag != nil {
		profile.TrustedDeviceFlag = *req.TrustedDeviceFlag
	}
	if req.StepUpAuthRequiredFlag != nil {
		profile.StepUpAuthRequiredFlag = *req.StepUpAuthRequiredFlag
	}

	s.appendAuditLocked(userID, "auth_profile_updated", "auth", "success", "Auth profile updated", "User")
	return *profile, nil
}

func (s *Service) KYCProfile(userID string) (KYCProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return KYCProfile{}, errors.New("user not found")
	}
	profile, ok := s.kycProfiles[userID]
	if !ok {
		return KYCProfile{}, errors.New("kyc profile not found")
	}
	return *profile, nil
}

func (s *Service) UpsertKYCProfile(userID string, req UpsertKYCProfileRequest) (KYCProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return KYCProfile{}, errors.New("user not found")
	}
	profile, ok := s.kycProfiles[userID]
	if !ok {
		profile = &KYCProfile{UserID: userID, FullName: user.Name, MobileNumber: user.Mobile, Email: user.Email, KYCStatus: "pending"}
		s.kycProfiles[userID] = profile
	}

	if value := strings.TrimSpace(req.FullName); value != "" {
		profile.FullName = value
		user.Name = value
	}
	if value := strings.TrimSpace(req.DateOfBirth); value != "" {
		profile.DateOfBirth = value
	}
	if value := strings.TrimSpace(req.Gender); value != "" {
		profile.Gender = value
	}
	if value := strings.TrimSpace(req.Email); value != "" {
		profile.Email = value
		user.Email = value
	}
	if value := strings.TrimSpace(req.PermanentAddress); value != "" {
		profile.PermanentAddress = value
	}
	if value := strings.TrimSpace(req.CommunicationAddress); value != "" {
		profile.CommunicationAddress = value
	}
	if value := strings.TrimSpace(req.PANMasked); value != "" {
		profile.PANMasked = value
	}
	if value := strings.TrimSpace(req.AadhaarMaskedOrHash); value != "" {
		profile.AadhaarMaskedOrHash = value
	}
	if len(req.KYCDocumentTypes) > 0 {
		profile.KYCDocumentTypes = append([]string(nil), req.KYCDocumentTypes...)
	}
	if len(req.KYCDocumentFiles) > 0 {
		profile.KYCDocumentFiles = append([]string(nil), req.KYCDocumentFiles...)
	}
	// H-9 fix: KYCStatus is NOT user-writable. Allowing it let a user set
	// KYCStatus:"verified" to unlock KYC-gated features without real verification.
	// Status transitions happen server-side only (see eKYC verification flow).
	if value := strings.TrimSpace(req.Verifier); value != "" {
		profile.Verifier = value
	}
	if value := strings.TrimSpace(req.Occupation); value != "" {
		profile.Occupation = value
	}
	if value := strings.TrimSpace(req.Employer); value != "" {
		profile.Employer = value
	}
	if value := strings.TrimSpace(req.AnnualIncome); value != "" {
		profile.AnnualIncome = value
	}
	if value := strings.TrimSpace(req.TaxResidency); value != "" {
		profile.TaxResidency = value
	}
	if value := strings.TrimSpace(req.NomineeDetails); value != "" {
		profile.NomineeDetails = value
	}
	if value := strings.TrimSpace(req.KYCVerifierID); value != "" {
		profile.KYCVerifierID = value
	}
	if value := strings.TrimSpace(req.KYCDocumentHashRef); value != "" {
		profile.KYCDocumentHashRef = value
	}
	if value := strings.TrimSpace(req.KYCRejectionReason); value != "" {
		profile.KYCRejectionReason = value
	}
	if value := strings.TrimSpace(req.KYCVerificationMethod); value != "" {
		profile.KYCVerificationMethod = value
	}

	now := time.Now().UTC()
	profile.KYCSubmissionDate = &now
	if strings.EqualFold(profile.KYCStatus, "verified") {
		user.EKYCVerified = true
	}

	s.appendAuditLocked(userID, "kyc_profile_upserted", "kyc", "success", "KYC profile updated", "User")
	return *profile, nil
}

func (s *Service) Accounts(userID string) ([]Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	accounts := append([]Account(nil), s.accounts[userID]...)
	return accounts, nil
}

func (s *Service) UpdateAccount(userID, accountID string, req UpdateAccountRequest) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return Account{}, errors.New("user not found")
	}
	accounts := s.accounts[userID]
	for i := range accounts {
		if accounts[i].ID != accountID {
			continue
		}
		if value := strings.TrimSpace(req.Nickname); value != "" {
			accounts[i].Nickname = value
		}
		if req.PrimaryAccountFlag != nil {
			if *req.PrimaryAccountFlag {
				for j := range accounts {
					accounts[j].PrimaryAccountFlag = false
				}
			}
			accounts[i].PrimaryAccountFlag = *req.PrimaryAccountFlag
		}
		if value := strings.TrimSpace(req.VerificationStatus); value != "" {
			accounts[i].VerificationStatus = value
		}
		if value := strings.TrimSpace(req.AccountVerificationMethod); value != "" {
			accounts[i].AccountVerificationMethod = value
		}
		now := time.Now().UTC()
		accounts[i].AccountVerificationAt = &now
		s.accounts[userID] = accounts
		agg := s.aggregatorStatus[userID]
		agg.NumberOfAccountsAggregated = len(accounts)
		agg.LastSyncTimestamp = &now
		if agg.ConnectorStatus == "" {
			agg.ConnectorStatus = "connected"
		}
		s.aggregatorStatus[userID] = agg
		s.appendAuditLocked(userID, "account_updated", "account_aggregator", "success", "Account metadata updated", "User")
		return accounts[i], nil
	}
	return Account{}, errors.New("account not found")
}

func (s *Service) CreateBeneficiary(userID string, req CreateBeneficiaryRequest) (Beneficiary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return Beneficiary{}, errors.New("user not found")
	}
	if strings.TrimSpace(req.BeneficiaryName) == "" || strings.TrimSpace(req.UPIIDOrBankDetails) == "" {
		return Beneficiary{}, errors.New("beneficiary name and upi/bank details are required")
	}
	now := time.Now().UTC()
	details := strings.TrimSpace(req.UPIIDOrBankDetails)

	// Verify the payee details against the bank provider (mock by default). The
	// status now reflects the real lookup result instead of always "pending":
	// a resolvable UPI/account is "verified"; an unknown one is "not_found".
	verificationStatus := "pending"
	if bank := s.bankAdapter(); bank != nil {
		if strings.Contains(details, "@") {
			if status, _, ok, err := bank.VerifyUPI(context.Background(), details); err == nil {
				if ok {
					verificationStatus = "verified"
				} else {
					verificationStatus = firstNonEmpty(status, "not_found")
				}
			}
		}
	}

	b := Beneficiary{
		ID:                      s.nextID("ben"),
		BeneficiaryName:         strings.TrimSpace(req.BeneficiaryName),
		RelationshipDescription: strings.TrimSpace(req.RelationshipDescription),
		UPIIDOrBankDetails:      details,
		DateAdded:               now,
		AddedBy:                 userID,
		VerificationStatus:      verificationStatus,
		TrustScore:              clamp(req.TrustScore, 0, 100),
		Notes:                   strings.TrimSpace(req.Notes),
		ApprovalInitiationID:    "approve_" + randomHex(6),
	}
	s.beneficiaryRecords[userID] = append(s.beneficiaryRecords[userID], b)
	s.appendAuditLocked(userID, "beneficiary_created", "beneficiary", "success",
		fmt.Sprintf("Beneficiary created (bank verification: %s)", verificationStatus), "User")
	return b, nil
}

func (s *Service) Beneficiaries(userID string) ([]Beneficiary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	out := append([]Beneficiary(nil), s.beneficiaryRecords[userID]...)
	return out, nil
}

func (s *Service) ApproveBeneficiary(userID, beneficiaryID string, req ApproveBeneficiaryRequest) (Beneficiary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return Beneficiary{}, errors.New("user not found")
	}
	list := s.beneficiaryRecords[userID]
	for i := range list {
		if list[i].ID != beneficiaryID {
			continue
		}
		now := time.Now().UTC()
		list[i].ApproverID = strings.TrimSpace(req.ApproverID)
		list[i].ApprovalTimestamp = &now
		list[i].CoolingActionID = strings.TrimSpace(req.CoolingActionID)
		if req.Approved {
			list[i].VerificationStatus = "approved"
			if _, ok := s.beneficiaries[userID]; !ok {
				s.beneficiaries[userID] = make(map[string]struct{})
			}
			s.beneficiaries[userID][strings.ToLower(list[i].BeneficiaryName)] = struct{}{}
			s.beneficiaries[userID][strings.ToLower(list[i].UPIIDOrBankDetails)] = struct{}{}
			s.appendAuditLocked(userID, "beneficiary_approved", "beneficiary", "success", "Beneficiary approved", firstNonEmpty(req.ApproverID, "System"))
		} else {
			list[i].VerificationStatus = "rejected"
			s.appendAuditLocked(userID, "beneficiary_approval_rejected", "beneficiary", "rejected", "Beneficiary approval rejected", firstNonEmpty(req.ApproverID, "System"))
		}
		s.beneficiaryRecords[userID] = list
		return list[i], nil
	}
	return Beneficiary{}, errors.New("beneficiary not found")
}

func (s *Service) AggregatorStatus(userID string) (AggregatorStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return AggregatorStatus{}, errors.New("user not found")
	}
	status := s.aggregatorStatus[userID]
	status.NumberOfAccountsAggregated = len(s.accounts[userID])
	if status.ConnectorStatus == "" {
		status.ConnectorStatus = "connected"
	}
	return status, nil
}

func (s *Service) SyncAggregator(userID string, req SyncAggregatorRequest) (AggregatorStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return AggregatorStatus{}, errors.New("user not found")
	}
	status := s.aggregatorStatus[userID]
	if value := strings.TrimSpace(req.LinkedInstitutionID); value != "" {
		status.LinkedInstitutionID = value
	}
	if value := strings.TrimSpace(req.ConnectorStatus); value != "" {
		status.ConnectorStatus = value
	} else if status.ConnectorStatus == "" {
		status.ConnectorStatus = "connected"
	}
	now := time.Now().UTC()
	status.LastSyncTimestamp = &now
	status.NumberOfAccountsAggregated = len(s.accounts[userID])
	s.aggregatorStatus[userID] = status
	s.appendAuditLocked(userID, "aggregator_sync_completed", "account_aggregator", "success", "Aggregator sync completed", "System")
	return status, nil
}

func (s *Service) nextID(prefix string) string {
	v := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("%s-%06d", prefix, v)
}

func (s *Service) seedPortfolio(userID string) {
	now := time.Now().UTC()
	s.investments[userID] = []InvestmentHolding{
		{ID: "holding-001", Name: "Nifty Index Fund", InstrumentName: "NIFTY 50 Index Fund", Category: "mutual_fund", QuantityUnits: 120.5, AvgPurchasePricePaise: 29875, CurrentValuePaise: 4200000, InvestedPaise: 3600000, LastValuationDate: now, SIPAmountPaise: 2500000, SIPFrequency: "monthly", SIPStartDate: now.AddDate(0, -6, 0), SIPStatus: "active", BrokerFundHouse: "HDFC AMC", DividendsPaise: 45000, CAGR: 12.8, CreatedAt: now},
		{ID: "holding-002", Name: "PPF", InstrumentName: "Public Provident Fund", Category: "ppf", QuantityUnits: 1, AvgPurchasePricePaise: 1700000, CurrentValuePaise: 1900000, InvestedPaise: 1700000, LastValuationDate: now, SIPAmountPaise: 125000, SIPFrequency: "yearly", SIPStartDate: now.AddDate(-2, 0, 0), SIPStatus: "active", BrokerFundHouse: "State Bank of India", DividendsPaise: 88000, CAGR: 7.1, CreatedAt: now},
		{ID: "holding-003", Name: "Gold ETF", InstrumentName: "SBI Gold ETF", Category: "gold", QuantityUnits: 55.0, AvgPurchasePricePaise: 16727, CurrentValuePaise: 1100000, InvestedPaise: 920000, LastValuationDate: now, SIPAmountPaise: 0, SIPFrequency: "", SIPStartDate: now, SIPStatus: "inactive", BrokerFundHouse: "SBI Mutual Fund", DividendsPaise: 0, CAGR: 8.5, CreatedAt: now},
	}
	s.insurance[userID] = []InsurancePolicy{
		{PolicyID: "POL-1001", PolicyType: "term_life", Insurer: "LIC", SumAssuredPaise: 100000000, PremiumPaise: 240000, NextDueDate: time.Now().AddDate(0, 1, 0).Format("2006-01-02")},
		{PolicyID: "POL-2001", PolicyType: "health", Insurer: "New India Assurance", SumAssuredPaise: 10000000, PremiumPaise: 65000, NextDueDate: time.Now().AddDate(0, 2, 0).Format("2006-01-02")},
	}
	s.loans[userID] = []LoanRecord{
		{LoanID: "LOAN-3001", Lender: "SBI", LoanType: "home", OutstandingPaise: 320000000, EMIPaise: 2850000, InterestRate: 8.55, RemainingMonths: 212},
		{LoanID: "LOAN-3002", Lender: "HDFC", LoanType: "car", OutstandingPaise: 4600000, EMIPaise: 165000, InterestRate: 9.20, RemainingMonths: 34},
	}
}

func (s *Service) appendAuditLocked(userID, eventType, resource, outcome, details, triggeredBy string) {
	item := AuditEvent{
		ID:          s.nextID("audit"),
		Timestamp:   time.Now().UTC(),
		UserID:      userID,
		EventType:   eventType,
		TriggeredBy: triggeredBy,
		Outcome:     outcome,
		Details:     details,
	}
	s.audit = append(s.audit, item)
	_, _ = s.ledger.WriteEvent(context.Background(), userID, eventType, resource, item)
}

// ErrInsufficientFunds is returned when a debit exceeds the user's total balance.
var ErrInsufficientFunds = errors.New("insufficient funds")

// totalBalanceLocked returns the sum of all account balances for a user.
func (s *Service) totalBalanceLocked(userID string) int64 {
	var total int64
	for _, a := range s.accounts[userID] {
		if a.BalancePaise > 0 {
			total += a.BalancePaise
		}
	}
	return total
}

// applyDebitToFirstAccountLocked debits amountPaise across the user's accounts.
// C-5 fix: it now verifies sufficient total balance FIRST and returns
// ErrInsufficientFunds instead of silently zeroing accounts and creating money
// out of nothing. Callers must check the error and not proceed on failure.
func (s *Service) applyDebitToFirstAccountLocked(userID string, amountPaise int64) error {
	if amountPaise <= 0 {
		return nil
	}
	if s.totalBalanceLocked(userID) < amountPaise {
		return ErrInsufficientFunds
	}
	accounts := s.accounts[userID]
	remaining := amountPaise
	for i := range accounts {
		if remaining <= 0 {
			break
		}
		if accounts[i].BalancePaise <= 0 {
			continue
		}
		if accounts[i].BalancePaise >= remaining {
			accounts[i].BalancePaise -= remaining
			remaining = 0
			break
		}
		remaining -= accounts[i].BalancePaise
		accounts[i].BalancePaise = 0
	}
	s.accounts[userID] = accounts
	return nil
}

func (s *Service) NetWorthSnapshot(userID string) (NetWorthSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return NetWorthSnapshot{}, errors.New("user not found")
	}
	now := time.Now().UTC()
	assets := s.buildNetWorthAssetsLocked(userID)
	liabilities := s.buildNetWorthLiabilitiesLocked(userID)
	var totalA, totalL int64
	for _, a := range assets {
		totalA += a.ValuePaise
	}
	for _, l := range liabilities {
		totalL += l.OutstandingPaise
	}
	trend := "up"
	if totalA < totalL {
		trend = "down"
	} else if totalA == totalL {
		trend = "flat"
	}
	return NetWorthSnapshot{
		Assets: assets, Liabilities: liabilities,
		TotalAssetsPaise: totalA, TotalLiabilitiesPaise: totalL,
		NetWorthPaise: totalA - totalL, TrendArrow: trend, ValuationDate: now,
	}, nil
}

func (s *Service) AddNetWorthAsset(userID string, req NetWorthAsset) (NetWorthAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return NetWorthAsset{}, errors.New("user not found")
	}
	req.ID = s.nextID("nwa")
	req.ValuationDate = time.Now().UTC()
	if req.ValuationSource == "" {
		req.ValuationSource = "user_entered"
	}
	s.netWorthAssets[userID] = append(s.netWorthAssets[userID], req)
	s.appendAuditLocked(userID, "networth_asset_added", "portfolio", "success", "Asset added", "User")
	if s.db != nil {
		saved := req
		db := s.db
		go func() { _ = db.SaveNetWorthAssetToDB(context.Background(), saved, userID) }()
	}
	return req, nil
}

func (s *Service) RemoveNetWorthAsset(userID, assetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	assets := s.netWorthAssets[userID]
	for i, a := range assets {
		if a.ID == assetID {
			s.netWorthAssets[userID] = append(assets[:i], assets[i+1:]...)
			return nil
		}
	}
	return errors.New("asset not found")
}

func (s *Service) MarketNews() []MarketNewsItem {
	s.mu.RLock()
	empty := len(s.marketNews) == 0
	s.mu.RUnlock()
	if empty {
		s.mu.Lock()
		s.seedMarketNews()
		s.mu.Unlock()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]MarketNewsItem(nil), s.marketNews...)
}

func (s *Service) PortfolioImpact(userID string) ([]PortfolioImpact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return nil, errors.New("user not found")
	}
	return []PortfolioImpact{
		{Symbol: "SENSEX", CurrentValue: 75180.42, ChangePercent: 0.42, ImpactPaise: int64(12450)},
		{Symbol: "NIFTY", CurrentValue: 22831.11, ChangePercent: 0.35, ImpactPaise: int64(8200)},
		{Symbol: "GOLD", CurrentValue: 74250.00, ChangePercent: -0.15, ImpactPaise: int64(-3200)},
	}, nil
}

func (s *Service) MarketNarration() MarketNarration {
	return MarketNarration{
		Text:          "Indian equities opened flat with a positive bias. Rate-sensitive sectors outperformed on expectations of stable policy rates. Gold retreated marginally as the dollar strengthened. Your portfolio's equity bias means a 1% Sensex move translates to approximately INR 850 in your holdings.",
		GeneratedAt:   time.Now().UTC(),
		Model:         "groq-llama3-8b",
		KeyIndicators: []string{"Sensex +0.42%", "Nifty +0.35%", "Gold -0.15%", "Repo Rate 6.50%"},
	}
}

func (s *Service) EmergencyContacts(userID string) []EmergencyContact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]EmergencyContact(nil), s.emergencyContacts[userID]...)
}

func (s *Service) AddEmergencyContact(userID string, req EmergencyContact) (EmergencyContact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return EmergencyContact{}, errors.New("user not found")
	}
	req.ID = s.nextID("emc")
	s.emergencyContacts[userID] = append(s.emergencyContacts[userID], req)
	s.appendAuditLocked(userID, "emergency_contact_added", "security", "success", "Emergency contact added", "User")
	if s.db != nil {
		saved := req
		db := s.db
		go func() { _ = db.SaveEmergencyContactToDB(context.Background(), saved, userID) }()
	}
	return req, nil
}

func (s *Service) DeleteEmergencyContact(userID, contactID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	contacts := s.emergencyContacts[userID]
	for i, c := range contacts {
		if c.ID == contactID {
			s.emergencyContacts[userID] = append(contacts[:i], contacts[i+1:]...)
			return nil
		}
	}
	return errors.New("emergency contact not found")
}

func (s *Service) SecurityTips() []SecurityTip {
	return []SecurityTip{
		{ID: "tip-1", Category: "phishing", Title: "Never share OTP or UPI PIN", Body: "Banks never ask for OTP, UPI PIN, or CVV over phone calls or SMS. Any such request is fraud."},
		{ID: "tip-2", Category: "links", Title: "Verify URLs before clicking", Body: "Hover over links to check the actual destination. Bank URLs always end with the official domain name."},
		{ID: "tip-3", Category: "device", Title: "Keep your device secure", Body: "Enable automatic OS updates and avoid installing apps from unknown sources."},
		{ID: "tip-4", Category: "sim", Title: "Watch for SIM swap signs", Body: "If your phone suddenly loses network and your UPI stops working, contact your bank immediately."},
	}
}

func (s *Service) DissolveGoal(userID, goalID string, req GoalDissolveRequest) (Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return Goal{}, errors.New("user not found")
	}
	goals := s.goals[userID]
	for i := range goals {
		if goals[i].ID == goalID {
			if goals[i].Status == "dissolved" {
				return Goal{}, errors.New("goal already dissolved")
			}
			goals[i].Status = "dissolved"
			if goals[i].SavedAmountPaise > 0 {
				acct := s.accounts[userID]
				if len(acct) > 0 {
					acct[0].BalancePaise += goals[i].SavedAmountPaise
					s.accounts[userID] = acct
				}
				goals[i].SavedAmountPaise = 0
			}
			computeGoalProgress(&goals[i])
			s.goals[userID] = goals
			s.appendAuditLocked(userID, "goal_dissolved", "goal", "success", "Goal dissolved", "User")
			return goals[i], nil
		}
	}
	return Goal{}, errors.New("goal not found")
}

func (s *Service) PauseGoal(userID, goalID string) (Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goals := s.goals[userID]
	for i := range goals {
		if goals[i].ID == goalID {
			if goals[i].Status != "active" {
				return Goal{}, errors.New("only active goals can be paused")
			}
			goals[i].Status = "paused"
			s.goals[userID] = goals
			return goals[i], nil
		}
	}
	return Goal{}, errors.New("goal not found")
}

func (s *Service) ResumeGoal(userID, goalID string) (Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goals := s.goals[userID]
	for i := range goals {
		if goals[i].ID == goalID {
			if goals[i].Status != "paused" {
				return Goal{}, errors.New("only paused goals can be resumed")
			}
			goals[i].Status = "active"
			s.goals[userID] = goals
			return goals[i], nil
		}
	}
	return Goal{}, errors.New("goal not found")
}

func (s *Service) GoalNudge(userID, goalID string, req GoalNudgeRequest) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.goals[userID] {
		if g.ID == goalID {
			switch req.NudgeType {
			case "contribution_reminder":
				return map[string]any{"message": fmt.Sprintf("You're %.0f%% toward %s. A small top-up now keeps you on track.", g.ProgressPercent, g.Name), "action": "contribute"}, nil
			case "deadline_warning":
				return map[string]any{"message": fmt.Sprintf("%s target date is approaching. Current shortfall: %s.", g.Name, FormatMoney(g.ShortfallEstimatePaise)), "action": "simulate"}, nil
			case "milestone_celebration":
				return map[string]any{"message": fmt.Sprintf("Congratulations! %s has passed %.0f%% completion.", g.Name, g.ProgressPercent), "action": "view"}, nil
			default:
				return map[string]any{"message": "Stay consistent with contributions to reach your goal on time.", "action": "contribute"}, nil
			}
		}
	}
	return nil, errors.New("goal not found")
}

func (s *Service) AvailableScenarios() []map[string]any {
	return []map[string]any{
		{"type": "expense_shock", "name": "Expense Shock", "description": "Simulate a sudden large expense (medical, repair, family event)", "params": []string{"amountPaise", "years"}},
		{"type": "interest_rate_hike", "name": "Interest Rate Hike", "description": "Repo rate increase impact on loan EMIs and debt investments", "params": []string{"rateIncreaseBps", "years"}},
		{"type": "job_loss", "name": "Job Loss / Income Interruption", "description": "Simulate 6-12 months of reduced or zero income", "params": []string{"incomeReductionPct", "months"}},
		{"type": "medical_emergency", "name": "Medical Emergency", "description": "Uninsured medical expense impact on net worth and goals", "params": []string{"amountPaise", "insuranceCoveragePaise"}},
		{"type": "goal_top_up", "name": "Goal Top-Up", "description": "What if you increased monthly contribution toward a specific goal?", "params": []string{"goalId", "additionalMonthlyPaise"}},
	}
}

func (s *Service) FeatureFlags(userID string) []FeatureFlag {
	s.mu.RLock()
	defer s.mu.RUnlock()
	flags, ok := s.featureFlags[userID]
	if !ok {
		return []FeatureFlag{
			{Key: "sse_events", Enabled: false},
			{Key: "voice_chatbot", Enabled: false},
			{Key: "crypto_portfolio", Enabled: false},
			{Key: "insurance_claims", Enabled: true},
			{Key: "dark_mode", Enabled: true},
		}
	}
	return append([]FeatureFlag(nil), flags...)
}

func (s *Service) NotificationCentre(userID string, page, limit int) ([]NotificationItem, error) {
	s.notifMu.Lock()
	defer s.notifMu.Unlock()
	items := s.notificationCenter[userID]
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * limit
	if start >= len(items) {
		return []NotificationItem{}, nil
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return append([]NotificationItem(nil), items[start:end]...), nil
}

func (s *Service) DismissNotification(userID, notificationID string) error {
	s.notifMu.Lock()
	defer s.notifMu.Unlock()
	items := s.notificationCenter[userID]
	for i := range items {
		if items[i].ID == notificationID {
			items[i].Read = true
			s.notificationCenter[userID] = items
			return nil
		}
	}
	return errors.New("notification not found")
}

func (s *Service) HelpArticles() []HelpArticle {
	return []HelpArticle{
		{ID: "help-1", Title: "How to Link a Bank Account", Category: "accounts", Content: "Go to Dashboard > Account Aggregator > Add Account. Enter your bank details and verify via UPI or micro-deposit."},
		{ID: "help-2", Title: "Understanding Your Health Score", Category: "health", Content: "Your financial health score is calculated across 7 pillars including savings rate, debt-to-income ratio, emergency fund adequacy, goal progress, and investment diversification. Green (700-900) is healthy, Amber (500-699) needs attention, Red (300-499) requires action."},
		{ID: "help-3", Title: "Emergency Freeze — When and How", Category: "security", Content: "If you suspect unauthorized access, long-press the Freeze button on the Dashboard for 2 seconds. Confirm via slider. All outgoing transactions are instantly frozen. Unfreezing requires biometric + OTP."},
		{ID: "help-4", Title: "Using the Tax Dashboard", Category: "tax", Content: "The tax page estimates your liability under old and new regimes using your linked account income and investment data. It highlights remaining deduction headroom and advance tax deadlines."},
	}
}

func (s *Service) PaginateList(list []map[string]any, page, limit int) ([]map[string]any, int) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	total := len(list)
	start := (page - 1) * limit
	if start >= total {
		return []map[string]any{}, total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return list[start:end], total
}

func (s *Service) seedMarketNews() {
	now := time.Now().UTC()
	s.marketNews = []MarketNewsItem{
		{ID: s.nextID("news"), Source: "RBI Bulletin", PublishTimestamp: now.Add(-2 * time.Hour), SentimentScore: 0.62, Headline: "RBI Keeps Repo Rate Unchanged at 6.50% for Sixth Consecutive Review", Summary: "The Monetary Policy Committee voted 5-1 to maintain status quo, citing balanced growth-inflation dynamics.", URL: "", FreshnessTimestamp: now},
		{ID: s.nextID("news"), Source: "SEBI", PublishTimestamp: now.Add(-4 * time.Hour), SentimentScore: 0.45, Headline: "SEBI Tightens F&O Rules to Curb Speculative Retail Trading", Summary: "New norms increase contract sizes and mandate upfront option premium collection.", URL: "", FreshnessTimestamp: now},
		{ID: s.nextID("news"), Source: "MoF", PublishTimestamp: now.Add(-6 * time.Hour), SentimentScore: 0.55, Headline: "Direct Tax Collections Rise 22% YoY in FY2025-26", Summary: "Net direct tax collections reached ₹18.9 lakh crore by mid-March, driven by strong advance tax payments.", URL: "", FreshnessTimestamp: now},
		{ID: s.nextID("news"), Source: "RBI", PublishTimestamp: now.Add(-8 * time.Hour), SentimentScore: -0.2, Headline: "RBI Flags Concerns Over Unsecured Retail Lending Growth", Summary: "Governance circular urges banks to strengthen underwriting standards for unsecured personal loans and credit cards.", URL: "", FreshnessTimestamp: now},
	}
}

func (s *Service) PushNotification(userID, category, title, body string) {
	s.notifMu.Lock()
	defer s.notifMu.Unlock()
	item := NotificationItem{
		ID:        s.nextID("notif"),
		Category:  category,
		Title:     title,
		Body:      body,
		Read:      false,
		CreatedAt: time.Now().UTC(),
	}
	items := s.notificationCenter[userID]
	items = append([]NotificationItem{item}, items...)
	if len(items) > 100 {
		items = items[:100]
	}
	s.notificationCenter[userID] = items
	// Also push to SSE stream
	go s.PublishSSE(userID, "notification", item)
}

// maxSSEConnectionsPerUser caps concurrent SSE streams per user (L-5) so a single
// user can't exhaust goroutines/memory by opening unbounded connections.
const maxSSEConnectionsPerUser = 5

// SubscribeSSE registers a new SSE subscriber for userID. It returns nil if the
// user already holds the maximum number of concurrent connections (L-5); the
// caller must treat a nil channel as "too many connections".
func (s *Service) SubscribeSSE(userID string) chan SSEEvent {
	s.sseMu.Lock()
	defer s.sseMu.Unlock()
	if len(s.sseSubscribers[userID]) >= maxSSEConnectionsPerUser {
		return nil
	}
	ch := make(chan SSEEvent, 16)
	s.sseSubscribers[userID] = append(s.sseSubscribers[userID], ch)
	return ch
}

func (s *Service) UnsubscribeSSE(userID string, ch chan SSEEvent) {
	s.sseMu.Lock()
	defer s.sseMu.Unlock()
	subs := s.sseSubscribers[userID]
	for i, sub := range subs {
		if sub == ch {
			s.sseSubscribers[userID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (s *Service) PublishSSE(userID, eventType string, data any) {
	s.sseMu.RLock()
	defer s.sseMu.RUnlock()
	for _, ch := range s.sseSubscribers[userID] {
		select {
		case ch <- SSEEvent{Event: eventType, Data: data}:
		default:
		}
	}
}

// PushTransactionEvent publishes a transaction event to the user's SSE stream.
func (s *Service) PushTransactionEvent(userID string, txn Transaction) {
	go s.PublishSSE(userID, "transaction", map[string]any{
		"id": txn.ID, "amountPaise": txn.AmountPaise,
		"recipient": txn.Recipient, "status": txn.Status,
		"riskLevel": txn.RiskLevel, "createdAt": txn.CreatedAt,
	})
}

// PushGoalEvent publishes a goal event to the user's SSE stream.
func (s *Service) PushGoalEvent(userID string, goal Goal, action string) {
	go s.PublishSSE(userID, "goal", map[string]any{
		"id": goal.ID, "name": goal.Name, "action": action,
		"progressPercent": goal.ProgressPercent, "status": goal.Status,
	})
}

func (s *Service) SystemStats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	txnCount := 0
	for _, txns := range s.transactions {
		txnCount += len(txns)
	}
	notifCount := 0
	for _, items := range s.notificationCenter {
		notifCount += len(items)
	}
	stats := s.fraudGraph.GetStats()
	return map[string]any{
		"user_count":            len(s.users),
		"transaction_count":     txnCount,
		"goal_count":            len(s.goals),
		"active_sessions":       len(s.sessions),
		"fraud_graph_nodes":     stats["total_nodes"],
		"fraud_graph_edges":     stats["total_edges"],
		"fraud_marked_nodes":    stats["fraud_nodes"],
		"notification_count":    notifCount,
		"sse_subscribers":       len(s.sseSubscribers),
		"circuit_breaker_state": string(s.dbCircuitBreaker.State()),
		"outbox_pending":        s.outbox.PendingCount(),
		"ledger_entries":        s.accountingLedger.EntryCount(),
	}
}

func (s *Service) DBPing(ctx context.Context) error {
	if s.db == nil {
		return errors.New("database not configured")
	}
	return s.db.repo.Ping(ctx)
}

func (s *Service) ReconcileBalances() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mismatches := 0
	accountsChecked := 0
	for userID, accts := range s.accounts {
		for _, acct := range accts {
			ledgerBalance := s.accountingLedger.GetBalance(acct.ID)
			accountsChecked++
			if acct.BalancePaise != ledgerBalance {
				mismatches++
			}
			_ = userID
		}
	}
	debits, credits, balanced := s.accountingLedger.VerifyBalance()
	return map[string]any{
		"accounts_checked":    accountsChecked,
		"balance_mismatches":  mismatches,
		"total_debits_paise":  debits,
		"total_credits_paise": credits,
		"ledger_balanced":     balanced,
		"entry_count":         s.accountingLedger.EntryCount(),
	}
}

func (s *Service) LedgerEntries(userID string, limit int) []AccountingLedgerEntry {
	return s.accountingLedger.GetEntriesByUser(userID, limit)
}

func (s *Service) buildNetWorthAssetsLocked(userID string) []NetWorthAsset {
	now := time.Now().UTC()
	assets := append([]NetWorthAsset(nil), s.netWorthAssets[userID]...)
	// auto-generate from accounts
	for _, acct := range s.accounts[userID] {
		assets = append(assets, NetWorthAsset{
			ID: fmt.Sprintf("sys-%s", acct.ID), Type: "bank_account",
			Description: acct.BankName + " " + acct.AccountType,
			ValuePaise:  acct.BalancePaise, ValuationDate: now, ValuationSource: "bank_provided",
		})
	}
	// auto-generate from investments
	for _, inv := range s.investments[userID] {
		assets = append(assets, NetWorthAsset{
			ID: fmt.Sprintf("sys-%s", inv.ID), Type: inv.Category,
			Description: inv.InstrumentName, ValuePaise: inv.CurrentValuePaise,
			ValuationDate: inv.LastValuationDate, ValuationSource: "market",
		})
	}
	return assets
}

func (s *Service) buildNetWorthLiabilitiesLocked(userID string) []NetWorthLiability {
	liabilities := make([]NetWorthLiability, 0)
	for _, loan := range s.loans[userID] {
		liabilities = append(liabilities, NetWorthLiability{
			ID: fmt.Sprintf("sys-%s", loan.LoanID), Type: loan.LoanType,
			Description: loan.Lender, LoanAmountPaise: loan.OutstandingPaise,
			OutstandingPaise: loan.OutstandingPaise, EMIPaise: loan.EMIPaise,
			InterestRate: loan.InterestRate, MaturityDate: time.Now().AddDate(0, loan.RemainingMonths, 0),
		})
	}
	return liabilities
}

func (s *Service) netWorthPaiseLocked(userID string) int64 {
	return s.totalAssetPaiseLocked(userID) - s.totalLiabilityPaiseLocked(userID)
}

func (s *Service) totalAssetPaiseLocked(userID string) int64 {
	var total int64
	for _, a := range s.accounts[userID] {
		total += a.BalancePaise
	}
	for _, inv := range s.investments[userID] {
		total += inv.CurrentValuePaise
	}
	for _, pol := range s.insurance[userID] {
		total += pol.SumAssuredPaise / 20
	}
	return total
}

func (s *Service) totalLiabilityPaiseLocked(userID string) int64 {
	var total int64
	for _, loan := range s.loans[userID] {
		total += loan.OutstandingPaise
	}
	return total
}

func (s *Service) healthScoreLocked(userID string) healthscore.Result {
	totalDebt := s.totalLiabilityPaiseLocked(userID)
	totalAssets := s.totalAssetPaiseLocked(userID)
	dti := 0.0
	if totalAssets > 0 {
		dti = float64(totalDebt) / float64(totalAssets+totalDebt)
	}

	emergencyFundMonths := 4.5
	savingsRate := 0.24
	goalOnTrack := 0.7
	if len(s.goals[userID]) == 0 {
		goalOnTrack = 0.5
	}

	res := s.healthCalculator.Calculate(healthscore.Input{
		EmergencyFundMonths:     emergencyFundMonths,
		DebtToIncomeRatio:       dti,
		SavingsRate:             savingsRate,
		DiversificationScore:    72,
		InsuranceCoverageScore:  68,
		GoalOnTrackRatio:        goalOnTrack,
		BehaviourQualityScore:   74,
		MonthlyHistoricalScores: []float64{62, 64, 66, 69, 71, 73},
	})
	return res
}

func historicalAverageAmount(history []Transaction) float64 {
	if len(history) == 0 {
		return 0
	}
	var total int64
	for _, tx := range history {
		total += tx.AmountPaise
	}
	return float64(total) / float64(len(history))
}

func velocityLastHour(history []Transaction, now time.Time) int {
	count := 0
	for _, tx := range history {
		if now.Sub(tx.CreatedAt) <= time.Hour {
			count++
		}
	}
	return count
}

func balanceImpact(history []Transaction, amountPaise int64, accounts []Account) float64 {
	var totalBalance int64
	for _, account := range accounts {
		totalBalance += account.BalancePaise
	}
	if totalBalance <= 0 {
		return 1
	}
	impact := float64(amountPaise) / float64(totalBalance)
	return clamp(impact, 0, 1)
}

func (s *Service) gnnScoreForRecipient(recipient string) float64 {
	if score, ok := s.muleScores[recipient]; ok {
		return score
	}
	if s.fraudGraph != nil {
		return s.fraudGraph.GetRiskScore(recipient)
	}
	// Fallback to pattern-based scoring if fraud graph is not initialized
	return fallbackGnnScore(recipient)
}

// rebuildMuleScores runs the ONNX mule GNN over all accounts and transactions
// and caches per-account P(mule). If the model or its node encoder is unavailable
// it clears the cache so gnnScoreForRecipient falls back to the FraudGraph.
func (s *Service) rebuildMuleScores() {
	// Skip the whole-graph rebuild unless the mule GNN can actually score (ONNX
	// build + model + node encoder all present); otherwise gnnScoreForRecipient
	// falls back to the FraudGraph and this per-transaction work is wasted.
	if s.mulePredictor == nil || !s.mulePredictor.IsAvailable() || !fraud.MuleEncoderReady() {
		return
	}
	input := s.buildMuleGraphInput()
	scores, err := fraud.ScoreMuleAccounts(s.mulePredictor, input)
	if err != nil || scores == nil {
		s.muleScores = make(map[string]float64)
		return
	}
	s.muleScores = scores
}

// buildMuleGraphInput assembles the mule-GNN input from live accounts and
// successful transfers. It computes the 9 graph-aggregate node features the
// training pipeline used (degrees, unique counterparties, incoming/outgoing
// amount sums+means, in RUPEES to match the training units) plus the available
// raw node fields; unavailable fields (avg_monthly_balance, monthly_income,
// occupation, account_status, home_*) are omitted so the encoder mean-defaults
// them. Encoding + scaling happen in fraud.encodeMuleNode via mule_preprocess.json.
func (s *Service) buildMuleGraphInput() fraud.MuleGraphInput {
	outCount := map[string]int{}
	inCount := map[string]int{}
	outSum := map[string]float64{}
	inSum := map[string]float64{}
	outRecips := map[string]map[string]struct{}{}
	inSenders := map[string]map[string]struct{}{}

	var transfers []fraud.MuleTransfer
	for uid, txns := range s.transactions {
		for _, t := range txns {
			if t.Status != "success" && t.Status != "" {
				continue
			}
			amt := float64(t.AmountPaise) / 100
			to := t.Recipient
			transfers = append(transfers, fraud.MuleTransfer{From: uid, To: to})

			outCount[uid]++
			outSum[uid] += amt
			if outRecips[uid] == nil {
				outRecips[uid] = map[string]struct{}{}
			}
			outRecips[uid][to] = struct{}{}

			inCount[to]++
			inSum[to] += amt
			if inSenders[to] == nil {
				inSenders[to] = map[string]struct{}{}
			}
			inSenders[to][uid] = struct{}{}
		}
	}

	var accounts []fraud.MuleAccount
	for uid, accs := range s.accounts {
		num := map[string]float64{}
		cat := map[string]string{}

		if user := s.users[uid]; user != nil {
			num["account_age_days"] = time.Now().UTC().Sub(user.CreatedAt).Hours() / 24.0
			if user.EKYCVerified {
				cat["kyc_status"] = "Verified"
			} else {
				cat["kyc_status"] = "Pending"
			}
		}
		if len(accs) > 0 {
			a := accs[0]
			if at := capitalizeFirst(a.AccountType); at != "" {
				cat["account_type"] = at
			}
			num["current_balance"] = float64(a.BalancePaise) / 100
		}

		od := float64(outCount[uid])
		ind := float64(inCount[uid])
		num["out_degree"] = od
		num["in_degree"] = ind
		num["total_degree"] = od + ind
		num["unique_receivers"] = float64(len(outRecips[uid]))
		num["unique_senders"] = float64(len(inSenders[uid]))
		num["total_outgoing_amount"] = outSum[uid]
		num["total_incoming_amount"] = inSum[uid]
		num["avg_outgoing_amount"] = 0
		if od > 0 {
			num["avg_outgoing_amount"] = outSum[uid] / od
		}
		num["avg_incoming_amount"] = 0
		if ind > 0 {
			num["avg_incoming_amount"] = inSum[uid] / ind
		}

		accounts = append(accounts, fraud.MuleAccount{ID: uid, Raw: num, Cat: cat})
	}

	return fraud.MuleGraphInput{Accounts: accounts, Transfers: transfers}
}

func envOrDefaultMulePath() string {
	p := os.Getenv("FINIX_MULE_MODEL_PATH")
	if p == "" {
		return "../../models/MuleAccountDetection.onnx"
	}
	return p
}

// wireMuleEncoder loads the mule node preprocessing artifact (mule_preprocess.json,
// alongside the ONNX model) and wires it into the fraud package. An absent or
// wrong-width artifact leaves the encoder unset so ScoreMuleAccounts fails safe.
func wireMuleEncoder() {
	path := preprocess.DefaultArtifactPath(envOrDefaultMulePath(), "mule_preprocess.json")
	art, err := preprocess.Load(path)
	if err != nil {
		slog.Info("mule preprocess artifact unavailable; mule GNN gated off, using fraud graph",
			"path", path, "error", err)
		return
	}
	fraud.SetMuleEncoder(art)
	if fraud.MuleEncoderReady() {
		slog.Info("mule node encoder wired", "path", path, "features", art.FeatureCount())
	} else {
		slog.Warn("mule preprocess artifact wrong width; mule GNN gated off", "features", art.FeatureCount())
	}
}

// capitalizeFirst upper-cases the first rune and lower-cases the rest, mapping
// backend casing (e.g. "savings") onto the training vocab (e.g. "Savings").
func capitalizeFirst(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func fallbackGnnScore(recipient string) float64 {
	r := strings.ToLower(strings.TrimSpace(recipient))
	switch {
	case strings.Contains(r, "unknown"), strings.Contains(r, "lottery"), strings.Contains(r, "urgent"):
		return 0.95
	case strings.Contains(r, "new"):
		return 0.65
	default:
		return 0.15
	}
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func shortenFingerprint(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:8])
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func randomHex(length int) string {
	if length <= 0 {
		return ""
	}
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

func (s *Service) estimateTaxableIncomeLocked(userID string) int64 {
	total := int64(120000000)
	for _, inv := range s.investments[userID] {
		gain := inv.CurrentValuePaise - inv.InvestedPaise
		if gain > 0 {
			total += gain
		}
	}
	return total
}

func (s *Service) taxSuggestionsLocked(userID string, regime string, currentTax int64) []string {
	suggestions := []string{}
	if regime == "new" {
		suggestions = append(suggestions, "Consider switching to old regime if you have significant 80C/80D deductions and home loan interest.")
	} else {
		suggestions = append(suggestions, "Maximize 80C with ELSS/PPF and 80D with health insurance premiums.")
	}
	if currentTax > 1000000*100 {
		suggestions = append(suggestions, "Advance tax payments due quarterly — ensure you've paid estimated tax to avoid interest under Section 234B/C.")
	}
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "Your estimated tax liability is minimal. Ensure TDS is correctly deducted.")
	}
	return suggestions
}

func (s *Service) detectPersonaLocked(userID string) string {
	if len(s.investments[userID]) > 5 {
		return "Active Investor"
	}
	if len(s.transactions[userID]) > 20 {
		return "Life-Event Driven"
	}
	return "Cautious Saver"
}

func (s *Service) estimateDeductionsLocked(userID string) []Deduction {
	// Derive 80C from ELSS/mutual fund holdings (equity-oriented investments)
	used80C := int64(0)
	for _, inv := range s.investments[userID] {
		if strings.Contains(strings.ToLower(inv.Category), "elss") || strings.Contains(strings.ToLower(inv.Category), "mutual") {
			used80C += inv.InvestedPaise
		}
	}
	if used80C == 0 {
		used80C = 5000000 // baseline estimate if no ELSS holdings
	}

	// Derive 24(b) home loan interest from loan records
	usedHomeLoan := int64(0)
	for _, loan := range s.loans[userID] {
		if strings.Contains(strings.ToLower(loan.LoanType), "home") {
			usedHomeLoan += loan.EMIPaise * int64(loan.RemainingMonths) / 12
		}
	}
	if usedHomeLoan == 0 {
		usedHomeLoan = 3000000
	}

	used80D := int64(2600000) // health insurance — estimated
	usedNPS := int64(1500000)

	limit80C := int64(1500000 * 100)
	limit80D := int64(100000 * 100)
	limitNPS := int64(50000 * 100)
	limitHomeLoan := int64(200000 * 100)

	return []Deduction{
		{Section: "80C", UsedPaise: minInt64(used80C, limit80C), LimitPaise: limit80C, RemainingPaise: maxInt64(limit80C-used80C, 0), Recommendation: "Use ELSS/PPF to close the remaining 80C room before March."},
		{Section: "80D", UsedPaise: minInt64(used80D, limit80D), LimitPaise: limit80D, RemainingPaise: maxInt64(limit80D-used80D, 0), Recommendation: "Top-up family floater health premium for additional deduction."},
		{Section: "80CCD(1B)", UsedPaise: minInt64(usedNPS, limitNPS), LimitPaise: limitNPS, RemainingPaise: maxInt64(limitNPS-usedNPS, 0), Recommendation: "NPS contribution can reduce taxable income further."},
		{Section: "24(b)", UsedPaise: minInt64(usedHomeLoan, limitHomeLoan), LimitPaise: limitHomeLoan, RemainingPaise: maxInt64(limitHomeLoan-usedHomeLoan, 0), Recommendation: "Home loan interest still has eligible headroom."},
	}
}

func FormatMoney(paise int64) string {
	return money.NewFromPaise(paise).Format()
}

// calculateOldRegimeTax computes income tax under old regime (FY 2025-26).
func calculateOldRegimeTax(taxableIncomePaise int64) int64 {
	if taxableIncomePaise <= 0 {
		return 0
	}
	income := taxableIncomePaise
	// Convert to rupee-based slabs (in paise)
	var tax int64
	switch {
	case income <= 250000*100:
		tax = 0
	case income <= 500000*100:
		tax = (income - 250000*100) * 5 / 100
	case income <= 1000000*100:
		tax = 12500*100 + (income-500000*100)*20/100
	default:
		tax = 12500*100 + 100000*100 + (income-1000000*100)*30/100
	}
	// Section 87A rebate if income <= 5L
	if income <= 500000*100 && tax > 0 {
		if tax <= 12500*100 {
			tax = 0
		} else {
			tax -= 12500 * 100
		}
	}
	// Surcharge
	surcharge := int64(0)
	switch {
	case income > 50000000*100:
		surcharge = tax * 37 / 100
	case income > 20000000*100:
		surcharge = tax * 25 / 100
	case income > 10000000*100:
		surcharge = tax * 15 / 100
	case income > 5000000*100:
		surcharge = tax * 10 / 100
	}
	// Health & Education Cess (4% on tax + surcharge)
	cess := (tax + surcharge) * 4 / 100
	return tax + surcharge + cess
}

// calculateNewRegimeTax computes income tax under new regime (FY 2025-26).
func calculateNewRegimeTax(taxableIncomePaise int64) int64 {
	if taxableIncomePaise <= 0 {
		return 0
	}
	income := taxableIncomePaise
	var tax int64
	switch {
	case income <= 300000*100:
		tax = 0
	case income <= 600000*100:
		tax = (income - 300000*100) * 5 / 100
	case income <= 900000*100:
		tax = 15000*100 + (income-600000*100)*10/100
	case income <= 1200000*100:
		tax = 15000*100 + 30000*100 + (income-900000*100)*15/100
	case income <= 1500000*100:
		tax = 15000*100 + 30000*100 + 45000*100 + (income-1200000*100)*20/100
	default:
		tax = 15000*100 + 30000*100 + 45000*100 + 60000*100 + (income-1500000*100)*30/100
	}
	// Section 87A rebate if income <= 7L
	if income <= 700000*100 && tax > 0 {
		if tax <= 25000*100 {
			tax = 0
		} else {
			tax -= 25000 * 100
		}
	}
	// Surcharge (same as old regime)
	surcharge := int64(0)
	switch {
	case income > 50000000*100:
		surcharge = tax * 37 / 100
	case income > 20000000*100:
		surcharge = tax * 25 / 100
	case income > 10000000*100:
		surcharge = tax * 15 / 100
	case income > 5000000*100:
		surcharge = tax * 10 / 100
	}
	cess := (tax + surcharge) * 4 / 100
	return tax + surcharge + cess
}
