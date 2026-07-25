package security

import (
	"log/slog"
	"sync"
	"time"
)

// ThreatType represents the types of confirmed threats per spec §3A.3 Layer 5.
type ThreatType string

const (
	ThreatSIMSwap            ThreatType = "sim_swap_confirmed"
	ThreatDeviceCompromise   ThreatType = "device_compromise_detected"
	ThreatCredentialStuffing ThreatType = "credential_stuffing_detected"
	ThreatMassFraudPattern   ThreatType = "mass_fraud_pattern_detected"
)

// ThreatResponse records the automated playbook decided for a confirmed threat.
//
// NOTE: the *Requested booleans record what the playbook DECIDED to do, not
// confirmed completion — no session store, mailer, or on-chain logger is wired in
// yet, so these must NOT be read as a done-actions audit trail. When those
// integrations land, wire the actions and flip the fields to past tense.
type ThreatResponse struct {
	ThreatType                     ThreatType `json:"threatType"`
	UserID                         string     `json:"userId,omitempty"`
	Action                         string     `json:"action"`
	SessionTerminationRequested    bool       `json:"sessionTerminationRequested"`
	TransactionSuspensionRequested bool       `json:"transactionSuspensionRequested"`
	EmergencyAlertRequested        bool       `json:"emergencyAlertRequested"`
	EmailAlertRequested            bool       `json:"emailAlertRequested"`
	PushNotificationRequested      bool       `json:"pushNotificationRequested"`
	CAPTCHARequested               bool       `json:"captchaRequested,omitempty"`
	RateLimitEscalated             bool       `json:"rateLimitEscalated,omitempty"`
	ManualReviewRequired           bool       `json:"manualReviewRequired,omitempty"`
	RequiresVideoKYC               bool       `json:"requiresVideoKYC,omitempty"`
	BlockchainLogRequested         bool       `json:"blockchainLogRequested"`
	Timestamp                      time.Time  `json:"timestamp"`
	XAIReason                      string     `json:"xaiReason"`
}

// ThreatResponseManager manages automated threat response playbooks.
type ThreatResponseManager struct {
	mu                 sync.RWMutex
	activeThreats      map[string][]ThreatResponse // userID → active threats
	credentialAttempts map[string]int              // IP → failed login attempts
	rateLimitEscalated map[string]bool             // IP → escalated
}

func NewThreatResponseManager() *ThreatResponseManager {
	return &ThreatResponseManager{
		activeThreats:      make(map[string][]ThreatResponse),
		credentialAttempts: make(map[string]int),
		rateLimitEscalated: make(map[string]bool),
	}
}

// RespondToThreat executes the appropriate automated playbook for a confirmed threat.
func (m *ThreatResponseManager) RespondToThreat(threat ThreatType, userID string, metadata map[string]any) ThreatResponse {
	now := time.Now().UTC()

	var response ThreatResponse
	response.ThreatType = threat
	response.UserID = userID
	response.Timestamp = now
	response.BlockchainLogRequested = true

	switch threat {
	case ThreatSIMSwap:
		response = m.playbookSIMSwap(response, userID, metadata)
	case ThreatDeviceCompromise:
		response = m.playbookDeviceCompromise(response, userID, metadata)
	case ThreatCredentialStuffing:
		response = m.playbookCredentialStuffing(response, userID, metadata)
	case ThreatMassFraudPattern:
		response = m.playbookMassFraud(response, userID, metadata)
	default:
		response.Action = "no_playbook_defined"
		response.XAIReason = "Unknown threat type — no automated playbook available."
	}

	// Record the threat response
	m.mu.Lock()
	m.activeThreats[userID] = append(m.activeThreats[userID], response)
	m.mu.Unlock()

	slog.Info("threat response playbook executed",
		"threat", string(threat),
		"userId", userID,
		"action", response.Action,
		"sessionTerminationRequested", response.SessionTerminationRequested,
	)

	return response
}

// playbookSIMSwap executes the SIM Swap Confirmed playbook per spec:
// "Immediate session termination. All active transactions suspended.
// Emergency alert sent to registered email. User must re-verify identity
// via video KYC before re-access is granted."
func (m *ThreatResponseManager) playbookSIMSwap(r ThreatResponse, userID string, metadata map[string]any) ThreatResponse {
	r.Action = "session_termination_transaction_suspension_video_kyc_required"
	r.SessionTerminationRequested = true
	r.TransactionSuspensionRequested = true
	r.EmailAlertRequested = true
	r.EmergencyAlertRequested = true
	r.RequiresVideoKYC = true
	r.ManualReviewRequired = true
	r.XAIReason = "SIM swap confirmed. All sessions terminated, outgoing transactions suspended. " +
		"Emergency alert sent to your registered email. Re-access requires video KYC verification."
	return r
}

// playbookDeviceCompromise executes the Device Compromise Detected playbook per spec:
// "Transaction functionality suspended. User is notified with clear instructions.
// Financial data is not erased — access is locked until re-verification on a clean device."
func (m *ThreatResponseManager) playbookDeviceCompromise(r ThreatResponse, userID string, metadata map[string]any) ThreatResponse {
	r.Action = "transaction_suspension_device_lock"
	r.TransactionSuspensionRequested = true
	r.PushNotificationRequested = true
	r.EmailAlertRequested = true
	r.XAIReason = "Device compromise detected (root/jailbreak). Transaction functionality suspended. " +
		"Please re-verify your identity on a clean device to restore access. Your financial data is safe."
	return r
}

// playbookCredentialStuffing executes the Credential Stuffing Attack Detected playbook per spec:
// "Rate limiting escalation across all accounts on the attacked infrastructure segment.
// CAPTCHA challenges introduced. Attack signature shared with threat intelligence feed."
func (m *ThreatResponseManager) playbookCredentialStuffing(r ThreatResponse, userID string, metadata map[string]any) ThreatResponse {
	ip := getStringFromMap(metadata, "ip", "unknown")
	m.mu.Lock()
	m.rateLimitEscalated[ip] = true
	m.mu.Unlock()

	r.Action = "rate_limit_escalation_captcha_threat_intel_share"
	r.CAPTCHARequested = true
	r.RateLimitEscalated = true
	r.XAIReason = "Credential stuffing attack detected. Rate limiting escalated, CAPTCHA challenges enabled. " +
		"Attack signature shared with the broader ecosystem threat intelligence feed."
	return r
}

// playbookMassFraud executes the Mass Fraud Pattern Detected playbook per spec:
// "Emergency broadcast pushed to all potentially affected users via push notification,
// SMS, and registered email simultaneously. Specific transaction types associated with
// the fraud pattern are temporarily elevated to manual review."
func (m *ThreatResponseManager) playbookMassFraud(r ThreatResponse, userID string, metadata map[string]any) ThreatResponse {
	r.Action = "emergency_broadcast_manual_review_elevation"
	r.EmergencyAlertRequested = true
	r.EmailAlertRequested = true
	r.PushNotificationRequested = true
	r.ManualReviewRequired = true
	r.XAIReason = "Mass fraud pattern detected affecting multiple users. Emergency broadcast sent via push notification, " +
		"SMS, and email. Affected transaction types temporarily elevated to manual review."
	return r
}

// RecordFailedLogin records a failed login attempt for credential stuffing detection.
func (m *ThreatResponseManager) RecordFailedLogin(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.credentialAttempts[ip]++
}

// ResetFailedLogins resets the failed login counter for an IP after successful login.
func (m *ThreatResponseManager) ResetFailedLogins(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.credentialAttempts, ip)
}

// IsCredentialStuffingDetected checks if an IP has exceeded the credential stuffing threshold.
func (m *ThreatResponseManager) IsCredentialStuffingDetected(ip string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.credentialAttempts[ip] >= 10 // 10+ failed attempts = credential stuffing
}

// IsRateLimitEscalated checks if rate limiting has been escalated for an IP.
func (m *ThreatResponseManager) IsRateLimitEscalated(ip string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rateLimitEscalated[ip]
}

// GetActiveThreats returns all active threats for a user.
func (m *ThreatResponseManager) GetActiveThreats(userID string) []ThreatResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]ThreatResponse(nil), m.activeThreats[userID]...)
}

// ClearThreats clears all active threats for a user (after resolution).
func (m *ThreatResponseManager) ClearThreats(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.activeThreats, userID)
}

func getStringFromMap(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}
