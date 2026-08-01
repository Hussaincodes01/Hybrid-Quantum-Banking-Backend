package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"FINIX/backend/internal/config"
	"FINIX/backend/internal/domain/credit"
	"FINIX/backend/internal/domain/debt"
	"FINIX/backend/internal/domain/security"
)

type EndpointInput struct {
	PathParams map[string]string
	Query      map[string]string
	Body       map[string]any
	Headers    map[string]string
}

type simBindingRecord struct {
	UserID     string
	Mobile     string
	DeviceID   string
	SimHash    string
	UBTToken   string
	Status     string
	VerifiedAt *time.Time
	RevokedAt  *time.Time
}

type upiPinRecord struct {
	PinHash        string
	FailedAttempts int
	LockedUntil    *time.Time
	UpdatedAt      time.Time
}

type expandedState struct {
	mu sync.RWMutex

	simBindings      map[string]simBindingRecord
	upiPins          map[string]upiPinRecord
	otpAttempts      map[string]int
	otpLockUntil     map[string]time.Time
	goalHistory      map[string][]map[string]any
	healthSnapshots  map[string][]map[string]any
	investmentOrders map[string][]map[string]any
	sipActions       map[string]map[string]any
	insuranceData    map[string][]map[string]any
	insuranceClaims  map[string][]map[string]any
	nomineeChanges   map[string][]map[string]any
	emergencyContact map[string][]map[string]any
	chatbotHistory   map[string][]map[string]any
	consentHistory   map[string][]map[string]any
	dataExports      map[string]map[string]any
	urlScans         map[string][]map[string]any
	simHistory       map[string][]map[string]any
	insightFeedback  map[string][]map[string]any
	blockchainData   map[string][]map[string]any
	freezeReason     map[string]string
	freezeAt         map[string]time.Time
	fraudForwarded   map[string]bool
	nudgeLog         map[string][]map[string]any
	sharingLog       map[string][]map[string]any
}

func newExpandedState() *expandedState {
	return &expandedState{
		simBindings:      make(map[string]simBindingRecord),
		upiPins:          make(map[string]upiPinRecord),
		otpAttempts:      make(map[string]int),
		otpLockUntil:     make(map[string]time.Time),
		goalHistory:      make(map[string][]map[string]any),
		healthSnapshots:  make(map[string][]map[string]any),
		investmentOrders: make(map[string][]map[string]any),
		sipActions:       make(map[string]map[string]any),
		insuranceData:    make(map[string][]map[string]any),
		insuranceClaims:  make(map[string][]map[string]any),
		nomineeChanges:   make(map[string][]map[string]any),
		emergencyContact: make(map[string][]map[string]any),
		chatbotHistory:   make(map[string][]map[string]any),
		consentHistory:   make(map[string][]map[string]any),
		dataExports:      make(map[string]map[string]any),
		urlScans:         make(map[string][]map[string]any),
		simHistory:       make(map[string][]map[string]any),
		insightFeedback:  make(map[string][]map[string]any),
		blockchainData:   make(map[string][]map[string]any),
		freezeReason:     make(map[string]string),
		freezeAt:         make(map[string]time.Time),
		fraudForwarded:   make(map[string]bool),
		nudgeLog:         make(map[string][]map[string]any),
		sharingLog:       make(map[string][]map[string]any),
	}
}

func (s *Service) VerifyBiometricChallenge(userID, challengeID, signatureB64 string, maxAge time.Duration) error {
	if err := s.requireUser(userID); err != nil {
		return err
	}
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return errors.New("biometric challenge ID is required")
	}
	signatureB64 = strings.TrimSpace(signatureB64)
	if signatureB64 == "" {
		return errors.New("biometric challenge signature is required")
	}

	s.mu.RLock()
	user := s.users[userID]
	s.mu.RUnlock()
	if user == nil || !user.BiometricEnabled {
		return errors.New("biometric step-up is not enabled")
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return errors.New("invalid base64 signature encoding")
	}

	// Look up the keyID from the user's auth profile
	s.mu.RLock()
	profile := s.authProfiles[userID]
	s.mu.RUnlock()
	if profile == nil || profile.DeviceChallengePublicKeyID == "" {
		return errors.New("no biometric key registered for step-up")
	}
	keyID := profile.DeviceChallengePublicKeyID

	valid, err := s.passkey.VerifyChallenge(userID, challengeID, signatureBytes, keyID)
	if err != nil {
		return fmt.Errorf("biometric verification failed: %w", err)
	}
	if !valid {
		return errors.New("biometric signature verification failed")
	}
	return nil
}

// CreateStepUpChallenge creates a passkey challenge for biometric step-up authentication.
// The client must sign the returned challenge with the device's biometric-unlocked private key
// and submit the signature via X-Biometric-Challenge + X-Biometric-Challenge-ID headers.
func (s *Service) CreateStepUpChallenge(userID string) (string, string, error) {
	if err := s.requireUser(userID); err != nil {
		return "", "", err
	}
	s.mu.RLock()
	user := s.users[userID]
	s.mu.RUnlock()
	if user == nil || !user.BiometricEnabled {
		return "", "", errors.New("biometric setup is required for step-up auth")
	}

	ch, err := s.passkey.CreateChallenge(userID)
	if err != nil {
		return "", "", fmt.Errorf("failed to create step-up challenge: %w", err)
	}

	return ch.ChallengeID, base64.StdEncoding.EncodeToString(ch.Challenge), nil
}

// CreateStepUpChallengeForAction is the M-3 action-bound variant of
// CreateStepUpChallenge: the returned challenge is cryptographically tied to
// `action` (e.g. "transaction_override:txn-123"), so the resulting signature
// only authorizes that exact operation. Pair it with
// VerifyBiometricChallengeForAction using the same action string.
func (s *Service) CreateStepUpChallengeForAction(userID, action string) (string, string, error) {
	if err := s.requireUser(userID); err != nil {
		return "", "", err
	}
	s.mu.RLock()
	user := s.users[userID]
	s.mu.RUnlock()
	if user == nil || !user.BiometricEnabled {
		return "", "", errors.New("biometric setup is required for step-up auth")
	}
	ch, err := s.passkey.CreateChallengeForAction(userID, strings.TrimSpace(action))
	if err != nil {
		return "", "", fmt.Errorf("failed to create step-up challenge: %w", err)
	}
	return ch.ChallengeID, base64.StdEncoding.EncodeToString(ch.Challenge), nil
}

// VerifyBiometricChallengeForAction is the M-3 action-bound verify: it requires
// the challenge to have been minted for `expectedAction`, rejecting a signature
// captured for a different (e.g. lower-value) operation and replayed here.
func (s *Service) VerifyBiometricChallengeForAction(userID, challengeID, signatureB64, expectedAction string, maxAge time.Duration) error {
	if err := s.requireUser(userID); err != nil {
		return err
	}
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return errors.New("biometric challenge ID is required")
	}
	signatureB64 = strings.TrimSpace(signatureB64)
	if signatureB64 == "" {
		return errors.New("biometric challenge signature is required")
	}

	s.mu.RLock()
	user := s.users[userID]
	s.mu.RUnlock()
	if user == nil || !user.BiometricEnabled {
		return errors.New("biometric step-up is not enabled")
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return errors.New("invalid base64 signature encoding")
	}

	s.mu.RLock()
	profile := s.authProfiles[userID]
	s.mu.RUnlock()
	if profile == nil || profile.DeviceChallengePublicKeyID == "" {
		return errors.New("no biometric key registered for step-up")
	}
	keyID := profile.DeviceChallengePublicKeyID

	valid, err := s.passkey.VerifyChallengeForAction(userID, challengeID, signatureBytes, keyID, strings.TrimSpace(expectedAction))
	if err != nil {
		return fmt.Errorf("biometric verification failed: %w", err)
	}
	if !valid {
		return errors.New("biometric signature verification failed")
	}
	return nil
}

// GenerateOTP creates a 6-digit OTP for the given user, stores it with a 10-minute expiry,
// and returns the OTP code. In production, this would trigger an SMS; in demo mode, the
// OTP is returned to the caller for test purposes.
func (s *Service) GenerateOTP(userID string) (string, error) {
	if err := s.requireUser(userID); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	profile, ok := s.authProfiles[userID]
	if !ok {
		return "", errors.New("auth profile not found")
	}

	// Check if locked out
	lockKey := "otp:" + userID
	if until, locked := s.exp.otpLockUntil[lockKey]; locked && time.Now().UTC().Before(until) {
		return "", fmt.Errorf("too many attempts, please try again later")
	}

	// Generate 6-digit code
	otpBytes := make([]byte, 6)
	if _, err := rand.Read(otpBytes); err != nil {
		return "", fmt.Errorf("failed to generate OTP: %w", err)
	}
	num := binary.BigEndian.Uint32(otpBytes[:4]) % 1000000
	otp := fmt.Sprintf("%06d", num)

	// Store OTP with 10-minute expiry
	now := time.Now().UTC()
	expiry := now.Add(10 * time.Minute)
	profile.OTPCode = otp
	profile.OTPExpiryTime = &expiry
	profile.OTPAttempts = 0

	s.exp.otpAttempts[lockKey] = 0
	delete(s.exp.otpLockUntil, lockKey)

	s.appendAuditLocked(userID, "otp_generated", "auth", "success", "OTP generated for step-up verification", "System")
	return otp, nil
}

func (s *Service) VerifyOTPToken(userID, otp string) error {
	if err := s.requireUser(userID); err != nil {
		return err
	}
	otp = strings.TrimSpace(otp)
	if otp == "" {
		return errors.New("otp token is required")
	}

	s.exp.mu.Lock()
	defer s.exp.mu.Unlock()

	lockKey := "otp:" + userID
	if until, ok := s.exp.otpLockUntil[lockKey]; ok && time.Now().UTC().Before(until) {
		return fmt.Errorf("too many attempts, please try again later")
	}

	s.mu.RLock()
	profile := s.authProfiles[userID]
	s.mu.RUnlock()
	if profile == nil {
		return errors.New("auth profile not found")
	}

	// Check OTP expiry
	if profile.OTPExpiryTime == nil || time.Now().UTC().After(*profile.OTPExpiryTime) {
		s.exp.otpAttempts[lockKey]++
		if s.exp.otpAttempts[lockKey] >= 3 {
			until := time.Now().UTC().Add(30 * time.Minute)
			s.exp.otpLockUntil[lockKey] = until
			s.exp.otpAttempts[lockKey] = 0
			return fmt.Errorf("too many attempts, please try again later")
		}
		return errors.New("otp has expired")
	}

	// Check OTP match. M-4: constant-time compare to avoid a timing oracle that
	// could let an attacker recover the OTP digit-by-digit. The whole check-and-
	// clear is already serialized under s.exp.mu, so a valid OTP can't be replayed
	// by two concurrent requests (the first clears OTPCode before the lock is
	// released).
	if subtle.ConstantTimeCompare([]byte(profile.OTPCode), []byte(otp)) != 1 {
		s.exp.otpAttempts[lockKey]++
		if s.exp.otpAttempts[lockKey] >= 3 {
			until := time.Now().UTC().Add(30 * time.Minute)
			s.exp.otpLockUntil[lockKey] = until
			s.exp.otpAttempts[lockKey] = 0
			return errors.New("too many attempts, OTP verification locked for 30 minutes")
		}
		return errors.New("invalid otp")
	}

	// Success — clear OTP and reset attempts
	s.mu.Lock()
	profile.OTPCode = ""
	profile.OTPExpiryTime = nil
	profile.OTPAttempts = 0
	s.mu.Unlock()

	s.exp.otpAttempts[lockKey] = 0
	delete(s.exp.otpLockUntil, lockKey)

	s.appendAuditLocked(userID, "otp_verified", "auth", "success", "OTP verification successful", "System")
	return nil
}

func (s *Service) CoolingOffState(userID, txID string) (bool, time.Duration, error) {
	if strings.TrimSpace(userID) == "" {
		return false, 0, errors.New("user id is required")
	}
	if err := s.requireUser(userID); err != nil {
		return false, 0, err
	}

	// On-chain enforcement is authoritative when the ledger is Fabric-backed:
	// the cooling-off window is derived from the committed transaction_blocked
	// events, which no application-layer state can override. If it reports an
	// active window we honour it immediately; otherwise we still check the
	// off-chain investment-order state below (not mirrored to the ledger).
	if s.LedgerIsFabricBacked() {
		if active, remaining := s.ledger.VerifyCoolingOff(userID, txID); active {
			return true, remaining, nil
		}
	}

	now := time.Now().UTC()
	s.mu.RLock()
	for _, tx := range s.transactions[userID] {
		if txID != "" && tx.ID != txID {
			continue
		}
		if tx.CoolingOffUntil != nil && tx.CoolingOffUntil.After(now) {
			remaining := tx.CoolingOffUntil.Sub(now)
			s.mu.RUnlock()
			return true, remaining, nil
		}
	}
	s.mu.RUnlock()

	s.exp.mu.RLock()
	orders := s.exp.investmentOrders[userID]
	for _, order := range orders {
		orderID := expString(order, "order_id")
		if txID != "" && orderID != txID {
			continue
		}
		if expString(order, "status") != "cooling_off" {
			continue
		}
		until, ok := order["cooling_off_until"].(time.Time)
		if !ok {
			continue
		}
		if until.After(now) {
			remaining := until.Sub(now)
			s.exp.mu.RUnlock()
			return true, remaining, nil
		}
	}
	s.exp.mu.RUnlock()

	return false, 0, nil
}

func (s *Service) HandleExpandedEndpoint(userID, endpointID string, input EndpointInput) (any, error) {
	if handled, resp, err := s.handleAuthAndUPIEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleDashboardEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleTransactionEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleGoalEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleHealthEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleInvestmentEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleInsuranceEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleLoanEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleSimulationEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleInsightsEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleSecurityEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleTaxEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleChatbotEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleSettingsEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleAuditEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleBlockchainEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleBeneficiarySupplementEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	if handled, resp, err := s.handleComplianceEndpoints(userID, endpointID, input); handled {
		return resp, err
	}
	return nil, fmt.Errorf("unknown endpoint id: %s", endpointID)
}

func (s *Service) handleAuthAndUPIEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "auth_sim_bind":
		mobile := strings.TrimSpace(expString(input.Body, "mobile"))
		deviceID := strings.TrimSpace(expString(input.Body, "device_id"))
		if mobile == "" || deviceID == "" {
			return true, nil, errors.New("mobile and device_id are required")
		}

		ubt := "ubt_" + randomHex(10)
		now := time.Now().UTC()
		simHash := expHash(mobile + ":" + deviceID)
		linkedUser := s.userIDByMobile(mobile)

		s.exp.mu.Lock()
		s.exp.simBindings[ubt] = simBindingRecord{
			UserID:     linkedUser,
			Mobile:     mobile,
			DeviceID:   deviceID,
			SimHash:    simHash,
			UBTToken:   ubt,
			Status:     "active",
			VerifiedAt: &now,
		}
		s.exp.mu.Unlock()

		if linkedUser != "" {
			s.recordExpandedAudit(linkedUser, "sim_bind_initiated", "auth", "SIM bind initiated with UBT")
		}
		return true, map[string]any{"ubt_token": ubt, "status": "active"}, nil
	case "auth_sim_bind_reverify":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		token := strings.TrimSpace(expString(input.Body, "ubt_token"))
		deviceID := strings.TrimSpace(expString(input.Body, "device_id"))
		if token == "" || deviceID == "" {
			return true, nil, errors.New("ubt_token and device_id are required")
		}
		now := time.Now().UTC()

		s.exp.mu.Lock()
		record, ok := s.exp.simBindings[token]
		if !ok {
			s.exp.mu.Unlock()
			return true, nil, errors.New("ubt token not found")
		}
		if record.UserID != "" && record.UserID != userID {
			s.exp.mu.Unlock()
			return true, nil, errors.New("forbidden for this user")
		}
		record.UserID = userID
		record.DeviceID = deviceID
		record.Status = "active"
		record.VerifiedAt = &now
		s.exp.simBindings[token] = record
		s.exp.mu.Unlock()

		// Check for SIM swap via the security manager, augmented by the external
		// SIM provider (mock by default; a real telecom API by env).
		currentSIMHash := security.HashSIMICCID(record.Mobile + ":" + deviceID)
		swapped := s.simBinding.DetectSIMSwap(userID, currentSIMHash)
		if v := s.simVerifier(); v != nil {
			if extSwapped, err := v.DetectSwap(context.Background(), userID); err == nil {
				swapped = swapped || extSwapped
			}
		}
		if swapped {
			s.threatResponse.RespondToThreat(security.ThreatSIMSwap, userID, map[string]any{
				"ubt_token": token,
			})
			s.recordExpandedAudit(userID, "sim_swap_detected", "auth", "SIM swap detected during re-verification")
			return true, map[string]any{"ubt_token": token, "status": "sim_swap_detected", "verified_at": now, "action": "sessions_terminated_video_kyc_required"}, nil
		}

		s.recordExpandedAudit(userID, "sim_bind_reverified", "auth", "UBT re-verification completed")
		return true, map[string]any{"ubt_token": token, "status": "verified", "verified_at": now}, nil
	case "auth_kin":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.mu.Lock()
		user := s.users[userID]
		if user != nil && strings.TrimSpace(user.KIN) == "" {
			user.KIN = "KIN-" + strings.ToUpper(randomHex(5))
		}
		kin := ""
		if user != nil {
			kin = user.KIN
		}
		s.mu.Unlock()
		return true, map[string]any{"kin": kin, "user_id": userID}, nil
	case "upi_pin_set", "upi_pin_reset":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		accountID := strings.TrimSpace(input.PathParams["accountID"])
		if accountID == "" {
			return true, nil, errors.New("accountID is required")
		}
		if !s.accountBelongsToUser(userID, accountID) {
			return true, nil, errors.New("account not found")
		}
		newPIN := strings.TrimSpace(expString(input.Body, "new_pin"))
		confirmPIN := strings.TrimSpace(expString(input.Body, "confirm_pin"))
		if newPIN == "" || confirmPIN == "" {
			return true, nil, errors.New("new_pin and confirm_pin are required")
		}
		if newPIN != confirmPIN {
			return true, nil, errors.New("new_pin and confirm_pin must match")
		}
		if len(newPIN) < 4 {
			return true, nil, errors.New("upi pin must be at least 4 digits")
		}
		if err := s.VerifyOTPToken(userID, expString(input.Body, "otp")); err != nil {
			return true, nil, err
		}
		if endpointID == "upi_pin_reset" {
			trustedDevice := strings.EqualFold(strings.TrimSpace(input.Headers["X-Device-Trusted"]), "true")
			if !trustedDevice {
				token := strings.TrimSpace(expString(input.Body, "ubt_token"))
				if token == "" {
					return true, nil, errors.New("ubt_token is required on unrecognized device")
				}
				s.exp.mu.RLock()
				_, ok := s.exp.simBindings[token]
				s.exp.mu.RUnlock()
				if !ok {
					return true, nil, errors.New("invalid ubt_token")
				}
			}
		}

		hash := expHash(newPIN + ":" + accountID)
		now := time.Now().UTC()
		s.exp.mu.Lock()
		rec := s.exp.upiPins[accountID]
		rec.PinHash = hash
		rec.FailedAttempts = 0
		rec.LockedUntil = nil
		rec.UpdatedAt = now
		s.exp.upiPins[accountID] = rec
		s.exp.mu.Unlock()

		eventType := "upi_pin_set"
		if endpointID == "upi_pin_reset" {
			eventType = "upi_pin_reset"
		}
		s.recordExpandedAudit(userID, eventType, "security", "UPI PIN updated")
		return true, map[string]any{
			"account_id": accountID,
			"status":     "updated",
			"meta": map[string]any{
				"screenshot_blocked": true,
			},
		}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleDashboardEndpoints(userID, endpointID string, _ EndpointInput) (bool, any, error) {
	switch endpointID {
	case "dashboard_net_worth_history":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		now := time.Now().UTC()
		s.mu.RLock()
		base := s.netWorthPaiseLocked(userID)
		s.mu.RUnlock()
		rows := make([]map[string]any, 0, 52)
		for i := 51; i >= 0; i-- {
			weekDate := now.AddDate(0, 0, -7*i)
			adjustment := int64((51 - i) * 3500 * 100)
			rows = append(rows, map[string]any{
				"week_start":      weekDate.Format("2006-01-02"),
				"net_worth_paise": base + adjustment,
			})
		}
		trend := "flat"
		if len(rows) >= 2 {
			first := rows[0]["net_worth_paise"].(int64)
			last := rows[len(rows)-1]["net_worth_paise"].(int64)
			if last > first {
				trend = "up"
			} else if last < first {
				trend = "down"
			}
		}
		return true, map[string]any{"snapshots": rows, "trend": trend}, nil
	case "dashboard_market_snapshot":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		market, err := s.MarketSnapshot(userID)
		if err != nil {
			return true, nil, err
		}
		market["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		market["refresh_interval_minutes"] = 15
		return true, market, nil
	case "security_freeze_status":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.mu.RLock()
		frozen := s.freezeState[userID]
		s.mu.RUnlock()
		s.exp.mu.RLock()
		reason := s.exp.freezeReason[userID]
		frozenAt := s.exp.freezeAt[userID]
		s.exp.mu.RUnlock()
		if reason == "" {
			reason = "user_initiated"
		}
		var frozenAtValue any
		if !frozenAt.IsZero() {
			frozenAtValue = frozenAt
		}
		return true, map[string]any{"frozen": frozen, "frozen_at": frozenAtValue, "reason": reason}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleTransactionEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "tx_cooling_off_status":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		txID := strings.TrimSpace(input.PathParams["txID"])
		if txID == "" {
			return true, nil, errors.New("txID is required")
		}
		active, remaining, err := s.CoolingOffState(userID, txID)
		if err != nil {
			return true, nil, err
		}
		options := []string{"cancel", "schedule", "override"}
		return true, map[string]any{
			"tx_id":              txID,
			"cooling_off_active": active,
			"remaining_seconds":  int(remaining.Seconds()),
			"options":            options,
		}, nil
	case "tx_schedule":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		txID := strings.TrimSpace(input.PathParams["txID"])
		if txID == "" {
			return true, nil, errors.New("txID is required")
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for i := range s.transactions[userID] {
			if s.transactions[userID][i].ID != txID {
				continue
			}
			scheduledFor := time.Now().UTC().Add(s.coolingOffWindow())
			s.transactions[userID][i].Status = "scheduled"
			s.transactions[userID][i].CoolingOffUntil = &scheduledFor
			s.appendAuditLocked(userID, "transaction_scheduled", "transaction", "success", "Blocked transaction scheduled after cooling window", "User")
			return true, map[string]any{"tx_id": txID, "status": "scheduled", "scheduled_for": scheduledFor}, nil
		}
		return true, nil, errors.New("transaction not found")
	case "tx_risk_score":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		txID := strings.TrimSpace(input.PathParams["txID"])
		if txID == "" {
			return true, nil, errors.New("txID is required")
		}
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, tx := range s.transactions[userID] {
			if tx.ID != txID {
				continue
			}
			return true, map[string]any{
				"tx_id":      tx.ID,
				"risk_level": tx.RiskLevel,
				"risk_score": tx.RiskScore,
				"breakdown": map[string]any{
					"amount_outlier":   0.72,
					"beneficiary_risk": s.fraudGraph.GetRiskScore(tx.Recipient),
					"session_trust":    0.84,
				},
				"xai_reason": tx.XAIReason,
			}, nil
		}
		return true, nil, errors.New("transaction not found")
	case "beneficiary_risk_check":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		recipient := expString(input.Body, "beneficiary")
		if recipient == "" {
			recipient = input.PathParams["beneficiaryID"]
		}
		if strings.TrimSpace(recipient) == "" {
			return true, nil, errors.New("beneficiary identifier is required")
		}
		score := s.fraudGraph.GetRiskScore(recipient)
		tier := "low"
		if score > 0.8 {
			tier = "high"
		} else if score > 0.5 {
			tier = "medium"
		}
		return true, map[string]any{
			"risk_tier":      tier,
			"graph_distance": int((1.0 - score) * 6),
			"flags":          []string{"recipient_graph_anomaly", "velocity_cluster"},
		}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleGoalEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "goals_recalculate":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		goalID := strings.TrimSpace(input.PathParams["goalID"])
		if goalID == "" {
			return true, nil, errors.New("goalID is required")
		}
		goal, err := s.GoalByID(userID, goalID)
		if err != nil {
			return true, nil, err
		}
		remaining := goal.TargetAmountPaise - goal.SavedAmountPaise
		if remaining < 0 {
			remaining = 0
		}
		monthsLeft := int(goal.TargetDate.Sub(time.Now().UTC()).Hours() / (24 * 30))
		if monthsLeft < 1 {
			monthsLeft = 1
		}
		newMonthly := remaining / int64(monthsLeft)

		event := map[string]any{
			"event_type": "recalculated",
			"created_at": time.Now().UTC(),
			"payload":    map[string]any{"new_monthly_paise": newMonthly},
		}
		s.exp.mu.Lock()
		hKey := userID + ":" + goalID
		s.exp.goalHistory[hKey] = append(s.exp.goalHistory[hKey], event)
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "goal_recalculated", "goal", "Goal allocation recalculated after missed contribution")

		return true, map[string]any{
			"goal_id":                  goalID,
			"new_monthly_allocation":   newMonthly,
			"extend_deadline_option":   goal.TargetDate.AddDate(0, 6, 0),
			"increase_contribution_by": int64(float64(newMonthly) * 0.18),
			"xai_explanation":          "Projected plan update considers remaining amount and current timeline.",
		}, nil
	case "goals_archived":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		goals := s.Goals(userID)
		archived := make([]Goal, 0)
		for _, goal := range goals {
			if goal.Status == "closed" || goal.Status == "completed" {
				archived = append(archived, goal)
			}
		}
		return true, map[string]any{"goals": archived}, nil
	case "goals_milestones":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		goalID := strings.TrimSpace(input.PathParams["goalID"])
		goal, err := s.GoalByID(userID, goalID)
		if err != nil {
			return true, nil, err
		}
		completion := 0.0
		if goal.TargetAmountPaise > 0 {
			completion = float64(goal.SavedAmountPaise) / float64(goal.TargetAmountPaise)
		}
		milestones := []string{}
		for _, pct := range []float64{0.25, 0.50, 0.75, 1.00} {
			if completion >= pct {
				milestones = append(milestones, fmt.Sprintf("%.0f%%", pct*100))
			}
		}
		return true, map[string]any{"goal_id": goalID, "milestones": milestones}, nil
	case "goals_history":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		goalID := strings.TrimSpace(input.PathParams["goalID"])
		hKey := userID + ":" + goalID
		s.exp.mu.RLock()
		events := append([]map[string]any(nil), s.exp.goalHistory[hKey]...)
		s.exp.mu.RUnlock()
		if len(events) == 0 {
			events = []map[string]any{{
				"event_type": "created",
				"created_at": time.Now().UTC().AddDate(0, -2, 0),
				"payload":    map[string]any{"goal_id": goalID},
			}}
		}
		sort.Slice(events, func(i, j int) bool {
			ti, _ := events[i]["created_at"].(time.Time)
			tj, _ := events[j]["created_at"].(time.Time)
			return ti.Before(tj)
		})
		return true, map[string]any{"goal_id": goalID, "events": events}, nil
	case "goals_market_impact_recalculate":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		goals := s.Goals(userID)
		affected := make([]map[string]any, 0, len(goals))
		for _, goal := range goals {
			if goal.Status != "active" {
				continue
			}
			affected = append(affected, map[string]any{
				"goal_id":                goal.ID,
				"old_target_date":        goal.TargetDate,
				"new_projected_date":     goal.TargetDate.AddDate(0, 2, 0),
				"market_drag_percentage": 3.8,
				"xai_explanation":        "Projected extension accounts for current market drawdown and expected monthly contribution pace.",
			})
		}
		s.recordExpandedAudit(userID, "goals_market_recalculated", "goal", "Active goals updated after market impact event")
		return true, map[string]any{"affected_goals": affected}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleHealthEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "health_pillars":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		res, err := s.HealthScore(userID)
		if err != nil {
			return true, nil, err
		}
		pillars := make([]map[string]any, 0, len(res.Pillars))
		for _, pillar := range res.Pillars {
			pillars = append(pillars, map[string]any{
				"name":            pillar.Name,
				"score":           pillar.Score,
				"weight":          pillar.Weight,
				"xai_explanation": fmt.Sprintf("%s score reflects your current financial behaviour trend and recent account activity.", pillar.Name),
			})
		}
		return true, map[string]any{"overall_score": res.Score300To900, "pillars": pillars}, nil
	case "health_history":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.exp.mu.Lock()
		if len(s.exp.healthSnapshots[userID]) == 0 {
			res, _ := s.HealthScore(userID)
			base := res.Score300To900 - 40
			for i := 11; i >= 0; i-- {
				ts := time.Now().UTC().AddDate(0, -i, 0)
				s.exp.healthSnapshots[userID] = append(s.exp.healthSnapshots[userID], map[string]any{
					"month": ts.Format("2006-01"),
					"score": base + (11-i)*4,
				})
			}
		}
		history := append([]map[string]any(nil), s.exp.healthSnapshots[userID]...)
		s.exp.mu.Unlock()
		return true, map[string]any{"snapshots": history}, nil
	case "health_simulate":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		scenario := strings.TrimSpace(expString(input.Body, "scenario_type"))
		if scenario == "" {
			scenario = "generic"
		}
		projected := map[string]any{
			"liquidity":         6,
			"debt_health":       14,
			"savings_behaviour": 8,
			"goal_alignment":    5,
		}
		if strings.Contains(strings.ToLower(scenario), "loan") {
			projected["debt_health"] = 20
		}
		result := map[string]any{
			"scenario_type":   scenario,
			"pillar_delta":    projected,
			"xai_explanation": "Projected delta estimates how this action may shift debt burden and savings resilience.",
		}
		s.exp.mu.Lock()
		s.exp.simHistory[userID] = append(s.exp.simHistory[userID], map[string]any{
			"simulation_type": "health-score",
			"input_params":    input.Body,
			"output_result":   result,
			"created_at":      time.Now().UTC(),
		})
		s.exp.mu.Unlock()
		return true, result, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleInvestmentEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "investment_detail":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		holdingID := strings.TrimSpace(input.PathParams["holdingID"])
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, holding := range s.investments[userID] {
			if holding.ID == holdingID {
				return true, map[string]any{
					"holding": holding,
					"metadata": map[string]any{
						"instrument_id":   "inst-" + strings.TrimPrefix(holding.ID, "holding-"),
						"xai_explanation": "Holding return profile uses invested value, current value, and category volatility assumptions.",
					},
				}, nil
			}
		}
		return true, nil, errors.New("holding not found")
	case "investment_order":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		amount := expInt64(input.Body, "amount")
		if amount <= 0 {
			return true, nil, errors.New("amount is required")
		}
		if amount > 25000*100 {
			challengeID := strings.TrimSpace(input.Headers["X-Biometric-Challenge-Id"])
			signature := strings.TrimSpace(input.Headers["X-Biometric-Challenge"])
			if err := s.VerifyBiometricChallenge(userID, challengeID, signature, 60*time.Second); err != nil {
				return true, nil, err
			}
		}

		orderType := firstNonEmpty(expString(input.Body, "order_type"), "buy")
		holdingID := strings.TrimSpace(expString(input.Body, "holding_id"))
		recipientRisk := s.fraudGraph.GetRiskScore(firstNonEmpty(expString(input.Body, "instrument"), holdingID))
		status := "executed"
		var coolingOffUntil any
		if recipientRisk > 0.8 {
			status = "cooling_off"
			coolingOffUntil = time.Now().UTC().Add(s.coolingOffWindow())
		}
		// M-13 fix: a BUY order moves real money out of the account. Verify funds and
		// debit BEFORE marking the order executed, so we can't report a phantom
		// execution with no corresponding debit. Done in its own s.mu critical section
		// (not nested inside s.exp.mu) to keep lock ordering simple. Skipped when the
		// order is held for cooling-off (no funds move yet).
		if strings.EqualFold(orderType, "buy") && status == "executed" {
			s.mu.Lock()
			debitErr := s.applyDebitToFirstAccountLocked(userID, amount)
			s.mu.Unlock()
			if debitErr != nil {
				return true, nil, debitErr
			}
		}
		orderID := "order-" + randomHex(8)
		order := map[string]any{
			"order_id":          orderID,
			"holding_id":        holdingID,
			"order_type":        orderType,
			"amount":            amount,
			"risk_score":        recipientRisk,
			"status":            status,
			"cooling_off_until": coolingOffUntil,
			"xai_explanation":   "Order risk uses transaction velocity and instrument-risk graph relationships.",
		}

		s.exp.mu.Lock()
		s.exp.investmentOrders[userID] = append(s.exp.investmentOrders[userID], order)
		s.appendBlockchainRecordLocked(userID, "investment_trade", orderID, order)
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "investment_order", "investment", "Investment order recorded")
		return true, order, nil
	case "investment_sip_start":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		sipID := "sip-" + randomHex(6)
		entry := map[string]any{
			"sip_id":       sipID,
			"action":       "start",
			"fund_id":      expString(input.Body, "fund_id"),
			"amount":       expInt64(input.Body, "amount"),
			"frequency":    firstNonEmpty(expString(input.Body, "frequency"), "monthly"),
			"start_date":   firstNonEmpty(expString(input.Body, "start_date"), time.Now().UTC().Format("2006-01-02")),
			"otp_verified": true,
		}
		s.exp.mu.Lock()
		s.exp.sipActions[sipID] = entry
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "sip_started", "investment", "SIP started")
		return true, entry, nil
	case "investment_sip_pause", "investment_sip_stop", "investment_sip_modify":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		if err := s.VerifyOTPToken(userID, expString(input.Body, "otp")); err != nil {
			return true, nil, err
		}
		sipID := strings.TrimSpace(input.PathParams["sipID"])
		if sipID == "" {
			return true, nil, errors.New("sipID is required")
		}
		action := "pause"
		if endpointID == "investment_sip_stop" {
			action = "stop"
		}
		if endpointID == "investment_sip_modify" {
			action = "modify"
		}
		s.exp.mu.Lock()
		rec, ok := s.exp.sipActions[sipID]
		if !ok {
			rec = map[string]any{"sip_id": sipID}
		}
		rec["action"] = action
		rec["updated_at"] = time.Now().UTC()
		rec["old_amount"] = rec["amount"]
		if action == "modify" {
			rec["amount"] = expInt64(input.Body, "new_amount")
		}
		rec["otp_verified"] = true
		s.exp.sipActions[sipID] = rec
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "sip_"+action, "investment", "SIP action updated")
		return true, rec, nil
	case "investment_liquidation_check":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.mu.RLock()
		var portfolioValue int64
		for _, holding := range s.investments[userID] {
			portfolioValue += holding.CurrentValuePaise
		}
		s.mu.RUnlock()
		s.exp.mu.RLock()
		var sessionSell int64
		for _, order := range s.exp.investmentOrders[userID] {
			if expString(order, "order_type") == "sell" {
				sessionSell += expInt64(order, "amount")
			}
		}
		s.exp.mu.RUnlock()
		ratio := 0.0
		if portfolioValue > 0 {
			ratio = float64(sessionSell) / float64(portfolioValue)
		}
		triggered := ratio > 0.30
		return true, map[string]any{
			"session_sell_ratio":          ratio,
			"cooling_off_required":        triggered,
			"cooling_off_window_in_hours": 6,
		}, nil
	case "investment_rebalance_suggestion":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{
			"suggestion": "Shift 5% from equity to debt to align with your target risk band.",
			"simulate_payload": map[string]any{
				"scenario_type": "portfolio_rebalance",
				"params":        map[string]any{"equity_delta": -5, "debt_delta": 5},
			},
			"xai_explanation": "Recommendation is based on current allocation drift and projected volatility over the next quarter.",
		}, nil
	case "investment_tax_loss_harvest":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{
			"opportunities": []map[string]any{
				{"holding_id": "holding-001", "estimated_savings_paise": 17500, "loss_paise": 92000},
				{"holding_id": "holding-003", "estimated_savings_paise": 9100, "loss_paise": 48000},
			},
		}, nil
	case "security_url_scan":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		url := strings.TrimSpace(expString(input.Body, "url"))
		if url == "" {
			return true, nil, errors.New("url is required")
		}
		verdict := "safe"
		flags := []string{}
		if strings.Contains(strings.ToLower(url), "bit.ly") || strings.Contains(strings.ToLower(url), "tinyurl") {
			verdict = "suspicious"
			flags = append(flags, "short_link")
		}
		if strings.Contains(strings.ToLower(url), "free-money") || strings.Contains(strings.ToLower(url), "urgent") {
			verdict = "blocked"
			flags = append(flags, "social_engineering_pattern")
		}
		result := map[string]any{
			"url":        url,
			"verdict":    verdict,
			"domain_age": "estimated",
			"flags":      flags,
		}
		s.exp.mu.Lock()
		s.exp.urlScans[userID] = append(s.exp.urlScans[userID], result)
		s.appendBlockchainRecordLocked(userID, "security_url_scan", "url-scan", result)
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "url_scanned", "security", "URL scan completed")
		return true, result, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleInsuranceEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "insurance_add":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		policyID := "policy-" + randomHex(6)
		policy := map[string]any{
			"policy_id":          policyID,
			"category":           firstNonEmpty(expString(input.Body, "category"), "life"),
			"insurer_name":       firstNonEmpty(expString(input.Body, "insurer_name"), "Demo Insurance"),
			"policy_number":      expString(input.Body, "policy_number"),
			"verified":           !strings.Contains(strings.ToLower(expString(input.Body, "policy_number")), "fake"),
			"created_at":         time.Now().UTC(),
			"xai_explanation":    "Policy verification uses issuer pattern checks and metadata consistency.",
			"screenshot_blocked": true,
		}
		s.exp.mu.Lock()
		s.exp.insuranceData[userID] = append(s.exp.insuranceData[userID], policy)
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "insurance_policy_added", "insurance", "Insurance policy added")
		return true, policy, nil
	case "insurance_detail":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		policyID := strings.TrimSpace(input.PathParams["policyID"])
		policy, ok := s.findInsurancePolicy(userID, policyID)
		if !ok {
			return true, nil, errors.New("policy not found")
		}
		return true, policy, nil
	case "insurance_delete":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		policyID := strings.TrimSpace(input.PathParams["policyID"])
		s.exp.mu.Lock()
		items := s.exp.insuranceData[userID]
		for i := range items {
			if expString(items[i], "policy_id") == policyID {
				items[i]["deleted_at"] = time.Now().UTC()
				s.exp.insuranceData[userID] = items
				s.exp.mu.Unlock()
				s.recordExpandedAudit(userID, "insurance_policy_deleted", "insurance", "Insurance policy soft deleted")
				return true, map[string]any{"policy_id": policyID, "status": "deleted"}, nil
			}
		}
		s.exp.mu.Unlock()
		return true, nil, errors.New("policy not found")
	case "insurance_nominee_change":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		if err := s.VerifyOTPToken(userID, expString(input.Body, "otp")); err != nil {
			return true, nil, err
		}
		if err := s.VerifyBiometricChallenge(userID, input.Headers["X-Biometric-Challenge-Id"], input.Headers["X-Biometric-Challenge"], 60*time.Second); err != nil {
			return true, nil, err
		}
		policyID := strings.TrimSpace(input.PathParams["policyID"])
		now := time.Now().UTC()
		entry := map[string]any{
			"policy_id":     policyID,
			"old_nominee":   expString(input.Body, "old_nominee"),
			"new_nominee":   expString(input.Body, "new_nominee"),
			"status":        "pending_cooloff",
			"cooloff_until": now.Add(24 * time.Hour),
			"otp_verified":  true,
			"bio_verified":  true,
		}
		s.exp.mu.Lock()
		s.exp.nomineeChanges[policyID] = append(s.exp.nomineeChanges[policyID], entry)
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "insurance_nominee_change", "insurance", "Nominee change initiated with cooloff")
		return true, entry, nil
	case "insurance_gap_analysis":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{
			"protection_score": 74,
			"gaps": []map[string]any{
				{"category": "life", "status": "adequate"},
				{"category": "health", "status": "needs_top_up"},
				{"category": "income_protection", "status": "underinsured"},
			},
			"xai_explanation": "Protection score blends policy coverage, premium continuity, and liability exposure.",
		}, nil
	case "insurance_protection_score":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"score": 74, "breakdown": map[string]any{"life": 81, "health": 67, "asset": 72, "income": 63}}, nil
	case "insurance_verify_document":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		fileName := strings.ToLower(expString(input.Body, "file_name"))
		verified := !strings.Contains(fileName, "fake")
		return true, map[string]any{"verified": verified, "confidence": 0.87, "flags": []string{"metadata_crosscheck"}}, nil
	case "insurance_claim_create":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		policyID := strings.TrimSpace(input.PathParams["policyID"])
		claimID := "claim-" + randomHex(7)
		claim := map[string]any{
			"claim_id":   claimID,
			"policy_id":  policyID,
			"claim_type": firstNonEmpty(expString(input.Body, "claim_type"), "general"),
			"status":     "initiated",
			"amount":     expInt64(input.Body, "amount"),
			"documents":  input.Body["documents"],
			"timeline":   []map[string]any{{"status": "initiated", "timestamp": time.Now().UTC(), "note": "Claim initiated"}},
			"created_at": time.Now().UTC(),
			"updated_at": time.Now().UTC(),
		}
		s.exp.mu.Lock()
		s.exp.insuranceClaims[policyID] = append(s.exp.insuranceClaims[policyID], claim)
		s.appendBlockchainRecordLocked(userID, "insurance_claim", claimID, claim)
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "insurance_claim_created", "insurance", "Insurance claim initiated")
		return true, claim, nil
	case "insurance_claims_list":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		policyID := strings.TrimSpace(input.PathParams["policyID"])
		s.exp.mu.RLock()
		claims := append([]map[string]any(nil), s.exp.insuranceClaims[policyID]...)
		s.exp.mu.RUnlock()
		return true, map[string]any{"claims": claims}, nil
	case "insurance_claim_detail":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		policyID := strings.TrimSpace(input.PathParams["policyID"])
		claimID := strings.TrimSpace(input.PathParams["claimID"])
		s.exp.mu.RLock()
		defer s.exp.mu.RUnlock()
		for _, claim := range s.exp.insuranceClaims[policyID] {
			if expString(claim, "claim_id") == claimID {
				return true, claim, nil
			}
		}
		return true, nil, errors.New("claim not found")
	case "insurance_claim_update":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		policyID := strings.TrimSpace(input.PathParams["policyID"])
		claimID := strings.TrimSpace(input.PathParams["claimID"])
		status := firstNonEmpty(expString(input.Body, "status"), "under_review")
		note := firstNonEmpty(expString(input.Body, "note"), "status updated")
		s.exp.mu.Lock()
		claims := s.exp.insuranceClaims[policyID]
		for i := range claims {
			if expString(claims[i], "claim_id") != claimID {
				continue
			}
			claims[i]["status"] = status
			tl, _ := claims[i]["timeline"].([]map[string]any)
			tl = append(tl, map[string]any{"status": status, "timestamp": time.Now().UTC(), "note": note})
			claims[i]["timeline"] = tl
			claims[i]["updated_at"] = time.Now().UTC()
			s.exp.insuranceClaims[policyID] = claims
			s.exp.mu.Unlock()
			s.recordExpandedAudit(userID, "insurance_claim_updated", "insurance", "Insurance claim status updated")
			return true, claims[i], nil
		}
		s.exp.mu.Unlock()
		return true, nil, errors.New("claim not found")
	case "insurance_network_hospitals":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		pincode := firstNonEmpty(input.Query["pincode"], "560001")
		return true, map[string]any{
			"pincode": pincode,
			"hospitals": []map[string]any{
				{"name": "City Care Hospital", "distance_km": 2.3, "cashless": true},
				{"name": "Metro Health Center", "distance_km": 4.7, "cashless": true},
			},
		}, nil
	case "insurance_premium_payment":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		policyID := strings.TrimSpace(input.PathParams["policyID"])
		amount := expInt64(input.Body, "amount")
		if amount <= 0 {
			return true, nil, errors.New("amount is required")
		}
		record := map[string]any{"policy_id": policyID, "amount": amount, "paid_at": time.Now().UTC()}
		s.exp.mu.Lock()
		s.appendBlockchainRecordLocked(userID, "insurance_premium", policyID, record)
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "insurance_premium_paid", "insurance", "Insurance premium payment recorded")
		return true, map[string]any{"status": "recorded", "policy_id": policyID, "amount": amount}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleLoanEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "loan_detail", "loan_prepayment_simulate", "loan_foreclosure_check", "loan_repayment_integrity":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		loanID := strings.TrimSpace(input.PathParams["loanID"])
		loan, ok := s.findLoan(userID, loanID)
		if !ok {
			return true, nil, errors.New("loan not found")
		}
		switch endpointID {
		case "loan_detail":
			principalShare := 0.62
			interestShare := 0.38
			return true, map[string]any{"loan": loan, "donut": map[string]any{"principal_share": principalShare, "interest_share": interestShare}}, nil
		case "loan_prepayment_simulate":
			extra := expInt64(input.Body, "extra_amount")
			if extra <= 0 {
				extra = loan.EMIPaise
			}
			extraMonth := int(expInt64(input.Body, "extra_month"))
			if extraMonth <= 0 {
				extraMonth = 1
			}
			res := debt.PrepaymentImpact(toDebtLoan(loan, inferFloating(loan.LoanType)), extra, extraMonth)
			return true, map[string]any{
				"loan_id":                 loanID,
				"interest_saved_paise":    res.InterestSavedPaise,
				"tenure_reduction":        res.TenureReducedMonths,
				"debt_free_date":          time.Now().UTC().AddDate(0, res.NewMonths, 0).Format("2006-01-02"),
				"baseline_months":         res.BaselineMonths,
				"new_months":              res.NewMonths,
				"baseline_interest_paise": res.BaselineInterest,
				"new_interest_paise":      res.NewInterest,
				"xai_explanation": fmt.Sprintf(
					"Paying an extra ₹%.2f at month %d clears the loan %d months sooner and saves ₹%.2f in interest (full amortisation, EMI held constant).",
					float64(extra)/100, extraMonth, res.TenureReducedMonths, float64(res.InterestSavedPaise)/100),
			}, nil
		case "loan_foreclosure_check":
			floating := inferFloating(loan.LoanType)
			if v, ok := input.Body["floating"]; ok {
				if b, ok2 := v.(bool); ok2 {
					floating = b
				}
			}
			amountUsed := loan.OutstandingPaise
			res := debt.ForeclosureDecision(toDebtLoan(loan, floating), amountUsed, s.params.Foreclosure)
			return true, map[string]any{
				"has_penalty":            res.HasPenalty,
				"penalty_amount":         res.PenaltyAmountPaise,
				"penalty_rate":           res.PenaltyRate,
				"interest_saved":         res.RemainingInterestPaise,
				"opportunity_cost_paise": res.OpportunityCostPaise,
				"net_benefit":            res.NetBenefitPaise,
				"recommended":            res.Recommended,
				"xai_reason":             res.Explanation,
			}, nil
		case "loan_repayment_integrity":
			return true, map[string]any{
				"loan_id":         loanID,
				"has_discrepancy": false,
				"matches":         12,
				"mismatches":      0,
			}, nil
		}
		return true, nil, errors.New("unsupported loan operation")
	case "loan_credit_utilisation":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		cards := parseCards(input.Body)
		rep := credit.EvaluateUtilisation(cards, time.Now().UTC(), s.params.Credit)
		return true, map[string]any{
			"utilisation_ratio":     rep.Aggregate, // computed aggregate (was constant 0.27)
			"alert":                 rep.AnyAlert,  // real T-lead breach, not ratio>0.28
			"aggregate_utilisation": rep.Aggregate,
			"per_card":              rep.Cards, // surfaces a maxed card masked by aggregate
			"threshold":             s.params.Credit.UtilisationAlert,
			"lead_days":             s.params.Credit.AlertLeadDays,
			"xai_reason": fmt.Sprintf(
				"Aggregate utilisation %.1f%% across %d card(s); a per-card breach within %d days of the statement date raises an alert (currently %v).",
				rep.Aggregate*100, len(cards), s.params.Credit.AlertLeadDays, rep.AnyAlert),
		}, nil
	case "loan_bnpl":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		obligations, apr := parseBNPLObligations(input.Body, s.params.Credit)
		out := make([]map[string]any, 0, len(obligations))
		for i, o := range obligations {
			row := map[string]any{
				"provider":         o.Provider,
				"due_amount_paise": o.DueAmountPaise,
				"due_date":         o.DueDate.Format("2006-01-02"),
			}
			if apr[i] > 0 {
				row["effective_apr"] = apr[i] // true annualised cost behind a "0%" headline
			}
			out = append(out, row)
		}
		return true, map[string]any{
			"obligations":             out,
			"total_outstanding_paise": credit.TotalBNPLOutstanding(obligations),
			"count":                   len(out),
		}, nil
	case "loan_optimisation_strategy":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.mu.RLock()
		recs := append([]LoanRecord(nil), s.loans[userID]...)
		s.mu.RUnlock()
		loans := make([]debt.Loan, 0, len(recs))
		for _, r := range recs {
			loans = append(loans, toDebtLoan(r, inferFloating(r.LoanType)))
		}
		extra := expInt64(input.Body, "extra_amount")
		if extra <= 0 {
			extra = 1000000 // ₹10,000 default monthly surplus
		}
		adherence := expFloat(input.Body, "adherence_score")
		res := debt.OptimiseStrategy(loans, extra, adherence, s.params.Debt)
		alt := debt.StrategySnowball
		if res.RecommendedStrategy == debt.StrategySnowball {
			alt = debt.StrategyAvalanche
		}
		return true, map[string]any{
			"recommended_strategy":            res.RecommendedStrategy,
			"xai_rationale":                   res.Explanation,
			"alternative":                     alt,
			"avalanche_interest_paise":        res.AvalancheInterestPaise,
			"snowball_interest_paise":         res.SnowballInterestPaise,
			"interest_cost_of_snowball_paise": res.InterestCostOfSnowball,
			"avalanche_months":                res.AvalancheMonths,
			"snowball_months":                 res.SnowballMonths,
		}, nil
	case "loan_rate_update_notify":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		newRepo := expFloat(input.Body, "repo_rate")
		if newRepo <= 0 {
			newRepo = 6.50
		}
		deltaBps := expFloat(input.Body, "delta_bps")
		if deltaBps == 0 {
			deltaBps = 25 // default +25 bps repo move
		}
		s.mu.RLock()
		recs := append([]LoanRecord(nil), s.loans[userID]...)
		s.mu.RUnlock()
		if len(recs) == 0 {
			return true, map[string]any{"repo_rate": newRepo, "new_projected_emi_paise": int64(0)}, nil
		}
		// Reprice the largest floating loan (most repo-sensitive).
		target := recs[0]
		for _, r := range recs {
			if r.OutstandingPaise > target.OutstandingPaise {
				target = r
			}
		}
		res := debt.RepoRateRecompute(toDebtLoan(target, true), deltaBps, s.params.Foreclosure.RepoTransmissionFactor)
		return true, map[string]any{
			"repo_rate":               newRepo,
			"new_projected_emi_paise": res.NewEMIPaise,
			"old_emi_paise":           res.OldEMIPaise,
			"old_rate_pct":            res.OldRatePct,
			"new_rate_pct":            res.NewRatePct,
			"delta_bps":               deltaBps,
			"loan_id":                 target.LoanID,
			"xai_explanation":         res.Explanation,
		}, nil
	case "loan_verify_payment_link":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		url := strings.ToLower(strings.TrimSpace(expString(input.Body, "url")))
		valid := strings.Contains(url, "sbi.co.in") || strings.Contains(url, "hdfcbank.com")
		return true, map[string]any{"verified": valid, "url": url, "xai_reason": "Link verification checks domain against lender allowlist."}, nil
	case "loan_debt_free_date":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.mu.RLock()
		loans := append([]LoanRecord(nil), s.loans[userID]...)
		s.mu.RUnlock()
		maxMonths := 0
		for _, loan := range loans {
			if loan.RemainingMonths > maxMonths {
				maxMonths = loan.RemainingMonths
			}
		}
		return true, map[string]any{"projected_debt_free_date": time.Now().UTC().AddDate(0, maxMonths, 0).Format("2006-01-02")}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleSimulationEndpoints(userID, endpointID string, _ EndpointInput) (bool, any, error) {
	switch endpointID {
	case "simulations_history":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.exp.mu.RLock()
		history := append([]map[string]any(nil), s.exp.simHistory[userID]...)
		s.exp.mu.RUnlock()
		return true, map[string]any{"history": history}, nil
	case "simulations_types":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		types := []map[string]any{
			{"id": "goal-topup", "title": "Goal Top-up Impact"},
			{"id": "loan-prepay", "title": "Loan Prepayment Impact"},
			{"id": "rebalance", "title": "Portfolio Rebalance Projection"},
			{"id": "tax-loss", "title": "Tax-Loss Harvest Projection"},
		}
		return true, map[string]any{"templates": types}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleInsightsEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "insights_persona_contest", "aiml_behaviour_persona_contest":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		reason := firstNonEmpty(expString(input.Body, "reason"), "user_requested_review")
		s.recordExpandedAudit(userID, "persona_contested", "aiml", "Persona contest submitted")
		return true, map[string]any{"status": "accepted", "reason": reason, "re_evaluation_eta_hours": 24}, nil
	case "insights_feedback":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		insightID := strings.TrimSpace(input.PathParams["insightID"])
		action := firstNonEmpty(expString(input.Body, "action"), "thumbs_up")
		entry := map[string]any{"insight_id": insightID, "action": action, "reason": expString(input.Body, "reason"), "created_at": time.Now().UTC()}
		s.exp.mu.Lock()
		s.exp.insightFeedback[userID] = append(s.exp.insightFeedback[userID], entry)
		s.exp.mu.Unlock()
		return true, map[string]any{"status": "recorded", "entry": entry}, nil
	case "insights_market_brief":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		hour := time.Now().Hour()
		period := "intraday"
		if hour < 11 {
			period = "morning"
		} else if hour > 16 {
			period = "evening"
		}
		return true, map[string]any{"period": period, "brief": "Markets are currently consolidating with selective sector rotation.", "xai_explanation": "Brief blends policy-rate stance, broad index trend, and sector leadership signals."}, nil
	case "insights_sector_heatmap":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"sectors": []map[string]any{{"name": "Banking", "change_pct": 1.2, "held": true}, {"name": "IT", "change_pct": -0.7, "held": false}, {"name": "Auto", "change_pct": 0.4, "held": true}}}, nil
	case "insights_ipo_calendar":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"ipos": []map[string]any{{"name": "FinEdge Tech", "open_date": "2026-05-02", "price_band": "₹420-₹450", "gmp": "₹36"}}}, nil
	case "insights_fund_compare":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		funds := strings.Split(firstNonEmpty(input.Query["funds"], "FundA,FundB"), ",")
		rows := make([]map[string]any, 0, len(funds))
		for _, fund := range funds {
			f := strings.TrimSpace(fund)
			if f == "" {
				continue
			}
			rows = append(rows, map[string]any{"fund": f, "return_1y": 12.1, "return_3y": 14.4, "return_5y": 11.8, "expense_ratio": 0.76, "sharpe": 1.2})
		}
		return true, map[string]any{"comparison": rows}, nil
	case "insights_rate_history":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		history := make([]map[string]any, 0, 20)
		for i := 20; i >= 0; i-- {
			history = append(history, map[string]any{"date": time.Now().UTC().AddDate(0, -3*i, 0).Format("2006-01-02"), "repo_rate": 5.25 + float64((i%5))/4})
		}
		return true, map[string]any{"history": history, "user_impact": "Projected floating-rate EMI may rise moderately if policy remains tight."}, nil
	case "insights_panic_check":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		volatility := expFloat(input.Body, "volatility_index")
		triggered := volatility >= 0.7
		return true, map[string]any{"triggered": triggered, "prompt": "Take 15 minutes before confirming this sell order and compare a simulation."}, nil
	case "insights_news_sources":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"sources": []string{"RBI", "SEBI", "NSE", "BSE", "Press Information Bureau"}}, nil
	case "insights_nudge_log":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.exp.mu.Lock()
		if len(s.exp.nudgeLog[userID]) == 0 {
			s.exp.nudgeLog[userID] = []map[string]any{{"message": "Top up emergency fund this month", "sent_at": time.Now().UTC().AddDate(0, 0, -2), "response": "dismissed"}}
		}
		rows := append([]map[string]any(nil), s.exp.nudgeLog[userID]...)
		s.exp.mu.Unlock()
		return true, map[string]any{"nudges": rows}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleSecurityEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "security_emergency_contacts_list":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.exp.mu.RLock()
		rows := append([]map[string]any(nil), s.exp.emergencyContact[userID]...)
		s.exp.mu.RUnlock()
		return true, map[string]any{"contacts": rows}, nil
	case "security_emergency_contacts_add":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		name := strings.TrimSpace(expString(input.Body, "name"))
		phone := strings.TrimSpace(expString(input.Body, "phone"))
		if name == "" || phone == "" {
			return true, nil, errors.New("name and phone are required")
		}
		entry := map[string]any{"id": "contact-" + randomHex(5), "name": name, "phone": phone, "relation": expString(input.Body, "relation"), "created_at": time.Now().UTC()}
		s.exp.mu.Lock()
		s.exp.emergencyContact[userID] = append(s.exp.emergencyContact[userID], entry)
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "emergency_contact_added", "security", "Emergency contact added")
		return true, entry, nil
	case "security_emergency_contacts_delete":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		contactID := strings.TrimSpace(input.PathParams["contactID"])
		s.exp.mu.Lock()
		rows := s.exp.emergencyContact[userID]
		for i := range rows {
			if expString(rows[i], "id") != contactID {
				continue
			}
			rows = append(rows[:i], rows[i+1:]...)
			s.exp.emergencyContact[userID] = rows
			s.exp.mu.Unlock()
			s.recordExpandedAudit(userID, "emergency_contact_deleted", "security", "Emergency contact deleted")
			return true, map[string]any{"status": "deleted", "contact_id": contactID}, nil
		}
		s.exp.mu.Unlock()
		return true, nil, errors.New("contact not found")
	case "security_tips":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		tips := []string{
			"Never share OTP, UPI PIN, or CVV over calls or messages.",
			"Verify payment links against official lender domains before paying.",
			"Use emergency freeze immediately if device compromise is suspected.",
		}
		s.exp.mu.RLock()
		recentScans := s.exp.urlScans[userID]
		s.exp.mu.RUnlock()
		if len(recentScans) > 0 {
			last := recentScans[len(recentScans)-1]
			if expString(last, "verdict") != "safe" {
				tips = append([]string{"Recent suspicious URL activity detected. Re-check unknown links before opening."}, tips...)
			}
		}
		return true, map[string]any{"tips": tips}, nil
	case "security_bank_contacts":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"contacts": []map[string]any{{"name": "SBI Helpline", "phone": "18001234"}, {"name": "NPCI", "phone": "18001201740"}, {"name": "National Cybercrime Helpline", "phone": "1930"}, {"name": "RBI Ombudsman", "phone": "14448"}}}, nil
	case "security_forward_nccp":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		reportID := strings.TrimSpace(expString(input.Body, "report_id"))
		if reportID == "" {
			return true, nil, errors.New("report_id is required")
		}
		s.exp.mu.Lock()
		s.exp.fraudForwarded[reportID] = true
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "fraud_report_forwarded", "security", "Fraud report forwarded to NCCP")
		return true, map[string]any{"report_id": reportID, "forwarded_nccp": true}, nil
	case "security_emergency_freeze_test":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"mode": "test", "frozen": false, "message": "Simulation complete. No live freeze applied."}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleTaxEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "tax_advance_schedule":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		year := time.Now().Year()
		schedule := []map[string]any{
			{"due_date": fmt.Sprintf("%d-06-15", year), "percent": 15, "amount_paise": 3500000},
			{"due_date": fmt.Sprintf("%d-09-15", year), "percent": 45, "amount_paise": 7000000},
			{"due_date": fmt.Sprintf("%d-12-15", year), "percent": 75, "amount_paise": 10500000},
			{"due_date": fmt.Sprintf("%d-03-15", year+1), "percent": 100, "amount_paise": 14000000},
		}
		return true, map[string]any{"schedule": schedule}, nil
	case "tax_pre_sale_check":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		units := expFloat(input.Body, "units")
		if units <= 0 {
			units = 1
		}
		return true, map[string]any{"stcg_paise": int64(units * 25000), "ltcg_paise": int64(units * 12000), "suggestion": "Waiting 18 days may shift this sale into lower projected tax bracket."}, nil
	case "tax_deductions_tracker":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"sections": []map[string]any{{"section": "80C", "used": 10800000, "limit": 15000000}, {"section": "80D", "used": 2600000, "limit": 5000000}, {"section": "80CCD", "used": 1500000, "limit": 5000000}, {"section": "24b", "used": 12000000, "limit": 20000000}, {"section": "HRA", "used": 6000000, "limit": 9000000}}}, nil
	case "tax_literacy_spending":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"breakdown": []map[string]any{{"sector": "Infrastructure", "pct": 24}, {"sector": "Healthcare", "pct": 16}, {"sector": "Education", "pct": 14}, {"sector": "Defense", "pct": 17}, {"sector": "Welfare", "pct": 29}}}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleChatbotEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "chatbot_history":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.exp.mu.Lock()
		if len(s.exp.chatbotHistory[userID]) == 0 {
			s.exp.chatbotHistory[userID] = []map[string]any{{
				"session_id": "chat-" + randomHex(6),
				"messages":   []map[string]any{{"role": "user", "content": "How is my risk profile?", "mode": "text", "created_at": time.Now().UTC().Add(-10 * time.Minute)}, {"role": "assistant", "content": "Your projected risk profile remains moderate.", "mode": "text", "created_at": time.Now().UTC().Add(-9 * time.Minute)}},
				"created_at": time.Now().UTC().Add(-10 * time.Minute),
			}}
		}
		rows := append([]map[string]any(nil), s.exp.chatbotHistory[userID]...)
		s.exp.mu.Unlock()
		return true, map[string]any{"sessions": rows}, nil
	case "chatbot_query_voice":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		audio := strings.TrimSpace(expString(input.Body, "audio_base64"))
		if audio == "" {
			return true, nil, errors.New("audio_base64 is required")
		}
		return true, map[string]any{"text": "Voice query processed. Your projected debt free date remains on track.", "tts_audio": ""}, nil
	case "chatbot_history_delete":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.exp.mu.Lock()
		delete(s.exp.chatbotHistory, userID)
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "chatbot_history_deleted", "chatbot", "Chatbot history deleted by user")
		return true, map[string]any{"status": "deleted"}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleSettingsEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "consent_pp_status":
		currentVersion := "2.1"
		resolvedUserID := strings.TrimSpace(userID)
		if resolvedUserID == "" {
			if token := bearerTokenFromExpandedHeaders(input.Headers); token != "" {
				deviceFP := strings.TrimSpace(input.Headers["X-Device-Fingerprint"])
				if uid, ok := s.AuthenticateToken(token, deviceFP); ok {
					resolvedUserID = uid
				}
			}
		}

		acceptedSetByQuery := false
		accepted := false
		if rawAccepted, ok := input.Query["accepted"]; ok {
			accepted = strings.EqualFold(strings.TrimSpace(rawAccepted), "true")
			acceptedSetByQuery = true
		}

		versionAccepted := strings.TrimSpace(input.Query["version_accepted"])

		if !acceptedSetByQuery && resolvedUserID != "" {
			s.mu.RLock()
			if userConsents, ok := s.consents[resolvedUserID]; ok {
				if consentGranted, exists := userConsents["privacy_policy"]; exists {
					accepted = consentGranted
				}
			}
			s.mu.RUnlock()
		}

		if versionAccepted == "" && accepted {
			versionAccepted = currentVersion
		}

		needsReaccept := !accepted || versionAccepted != currentVersion
		response := map[string]any{
			"accepted":        accepted,
			"current_version": currentVersion,
			"needs_reaccept":  needsReaccept,
		}

		if accepted {
			response["version_accepted"] = versionAccepted
		}

		if accepted && needsReaccept {
			response["changes_summary"] = "Updated data retention periods and added blockchain consent records."
		}

		return true, response, nil
	case "settings_data_export_create":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		exportID := "export-" + randomHex(8)
		format := firstNonEmpty(expString(input.Body, "format"), "pdf")
		rec := map[string]any{
			"export_id":  exportID,
			"user_id":    userID,
			"format":     format,
			"status":     "processing",
			"created_at": time.Now().UTC(),
			"expires_at": time.Now().UTC().Add(24 * time.Hour),
		}
		s.exp.mu.Lock()
		s.exp.dataExports[exportID] = rec
		s.exp.mu.Unlock()
		s.recordExpandedAudit(userID, "data_export_requested", "settings", "User requested data export")
		return true, map[string]any{"export_id": exportID, "status": "processing"}, nil
	case "settings_data_export_status":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		exportID := strings.TrimSpace(input.PathParams["exportID"])
		s.exp.mu.Lock()
		rec, ok := s.exp.dataExports[exportID]
		if !ok {
			s.exp.mu.Unlock()
			return true, nil, errors.New("export not found")
		}
		createdAt, _ := rec["created_at"].(time.Time)
		if time.Since(createdAt) > 2*time.Second {
			rec["status"] = "ready"
			rec["download_url"] = "/downloads/" + exportID
			s.exp.dataExports[exportID] = rec
		}
		s.exp.mu.Unlock()
		return true, rec, nil
	case "settings_data_sharing_log":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.exp.mu.Lock()
		if len(s.exp.sharingLog[userID]) == 0 {
			s.exp.sharingLog[userID] = []map[string]any{{"recipient": "Credit Bureau", "purpose": "loan underwriting", "timestamp": time.Now().UTC().AddDate(0, 0, -12)}}
		}
		rows := append([]map[string]any(nil), s.exp.sharingLog[userID]...)
		s.exp.mu.Unlock()
		return true, map[string]any{"events": rows}, nil
	case "settings_data_retention":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"policies": []map[string]any{{"category": "transactions", "retention": "8 years"}, {"category": "chatbot", "retention": "1 year"}, {"category": "consent", "retention": "as mandated"}}}, nil
	case "settings_consent_history":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		s.exp.mu.Lock()
		if len(s.exp.consentHistory[userID]) == 0 {
			for k, v := range s.consents[userID] {
				action := "revoked"
				if v {
					action = "granted"
				}
				s.exp.consentHistory[userID] = append(s.exp.consentHistory[userID], map[string]any{"consent_type": k, "action": action, "created_at": time.Now().UTC().AddDate(0, 0, -7)})
			}
		}
		rows := append([]map[string]any(nil), s.exp.consentHistory[userID]...)
		s.exp.mu.Unlock()
		return true, map[string]any{"events": rows}, nil
	case "settings_grievance_contact":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"officer": "Grievance Officer", "email": "grievance@FINIX.example", "phone": "+91-1800-111-222"}, nil
	case "settings_transparency_report":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"year": time.Now().Year() - 1, "model_updates": 12, "fraud_flags": 18231, "false_positive_rate": 0.018}, nil
	case "settings_ethical_charter":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"title": "Ethical AI Charter", "principles": []string{"Fairness and bias monitoring", "Human override on high-risk decisions", "Explainability before automation", "Privacy by design"}}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleAuditEndpoints(userID, endpointID string, _ EndpointInput) (bool, any, error) {
	switch endpointID {
	case "audit_logs_export":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		exportID := "audit-export-" + randomHex(8)
		s.recordExpandedAudit(userID, "audit_export_requested", "audit", "Audit export requested")
		return true, map[string]any{"export_id": exportID, "status": "processing"}, nil
	case "audit_logs_integrity_proof":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		proof, err := s.AuditLogIntegrity(userID)
		if err != nil {
			return true, nil, err
		}
		return true, map[string]any{"merkle_root": proof.MerkleRoot, "event_count": proof.EventCount}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleBlockchainEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "blockchain_consent_record", "blockchain_insurance_claim", "blockchain_loan_repayment", "blockchain_investment_trade":
		targetUser := strings.TrimSpace(expString(input.Body, "user_id"))
		if targetUser == "" {
			targetUser = userID
		}
		if strings.TrimSpace(targetUser) == "" {
			return true, nil, errors.New("user_id is required")
		}
		referenceID := firstNonEmpty(expString(input.Body, "consent_event_id"), expString(input.Body, "claim_id"))
		if referenceID == "" {
			referenceID = firstNonEmpty(expString(input.Body, "order_id"), expString(input.Body, "loan_id"))
		}
		recordType := map[string]string{
			"blockchain_consent_record":   "consent",
			"blockchain_insurance_claim":  "insurance_claim",
			"blockchain_loan_repayment":   "loan_repayment",
			"blockchain_investment_trade": "investment_trade",
		}[endpointID]
		payload := map[string]any{"reference_id": referenceID, "record_type": recordType, "requested_at": time.Now().UTC()}
		s.exp.mu.Lock()
		item := s.appendBlockchainRecordLocked(targetUser, recordType, referenceID, payload)
		s.exp.mu.Unlock()
		return true, map[string]any{"tx_hash": item["tx_hash"], "block_number": item["block_number"]}, nil
	case "blockchain_news_whitelist":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		return true, map[string]any{"sources": []string{"RBI", "SEBI", "NSE", "BSE", "Economic Times"}}, nil
	case "blockchain_cooling_off":
		txID := strings.TrimSpace(input.PathParams["txID"])
		targetUser := firstNonEmpty(expString(input.Body, "user_id"), userID)
		if targetUser == "" {
			return true, nil, errors.New("user_id is required")
		}
		active, remaining, err := s.CoolingOffState(targetUser, txID)
		if err != nil {
			return true, nil, err
		}
		return true, map[string]any{"tx_id": txID, "active": active, "remaining_seconds": int(remaining.Seconds())}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleBeneficiarySupplementEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "beneficiary_delete":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		beneficiaryID := strings.TrimSpace(input.PathParams["beneficiaryID"])
		s.mu.Lock()
		list := s.beneficiaryRecords[userID]
		for i := range list {
			if list[i].ID == beneficiaryID {
				list[i].VerificationStatus = "deleted"
				s.beneficiaryRecords[userID] = list
				s.appendAuditLocked(userID, "beneficiary_deleted", "beneficiary", "success", "Beneficiary soft deleted", "User")
				s.mu.Unlock()
				return true, map[string]any{"beneficiary_id": beneficiaryID, "status": "deleted"}, nil
			}
		}
		s.mu.Unlock()
		return true, nil, errors.New("beneficiary not found")
	case "beneficiary_detail":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		beneficiaryID := strings.TrimSpace(input.PathParams["beneficiaryID"])
		list, err := s.Beneficiaries(userID)
		if err != nil {
			return true, nil, err
		}
		for _, item := range list {
			if item.ID == beneficiaryID {
				score := s.fraudGraph.GetRiskScore(item.BeneficiaryName)
				flags := []string{}
				if score > 0.5 {
					flags = append(flags, "graph_proximity_anomaly")
				}
				return true, map[string]any{"beneficiary": item, "gnn_risk_flags": flags}, nil
			}
		}
		return true, nil, errors.New("beneficiary not found")
	case "beneficiaries_recent":
		if err := s.requireUser(userID); err != nil {
			return true, nil, err
		}
		history := s.TransactionHistory(userID)
		seen := make(map[string]bool)
		recent := make([]map[string]any, 0, 5)
		for _, tx := range history {
			recipient := strings.TrimSpace(tx.Recipient)
			if recipient == "" || seen[recipient] {
				continue
			}
			seen[recipient] = true
			recent = append(recent, map[string]any{"name": recipient, "last_paid_at": tx.CreatedAt, "amount_paise": tx.AmountPaise})
			if len(recent) == 5 {
				break
			}
		}
		return true, map[string]any{"beneficiaries": recent}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) handleComplianceEndpoints(userID, endpointID string, input EndpointInput) (bool, any, error) {
	switch endpointID {
	case "compliance_breach_notify":
		incident := expString(input.Body, "incident_details")
		if strings.TrimSpace(incident) == "" {
			return true, nil, errors.New("incident_details is required")
		}
		return true, map[string]any{"status": "notified", "reference": "dpdp-" + randomHex(6), "affected_users": expInt64(input.Body, "affected_users")}, nil
	case "aiml_bias_audit":
		return true, map[string]any{
			"quarter": "Q1",
			"models": []map[string]any{
				{"name": "transaction-risk-v3", "bias_score": 0.07, "status": "within_threshold"},
				{"name": "persona-v2", "bias_score": 0.05, "status": "within_threshold"},
			},
		}, nil
	default:
		return false, nil, nil
	}
}

func (s *Service) requireUser(userID string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("unauthorized")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.users[userID]; !ok {
		return errors.New("user not found")
	}
	return nil
}

func (s *Service) userIDByMobile(mobile string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mobileIndex[mobile]
}

func (s *Service) accountBelongsToUser(userID, accountID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, account := range s.accounts[userID] {
		if account.ID == accountID {
			return true
		}
	}
	return false
}

func (s *Service) recordExpandedAudit(userID, eventType, resource, details string) {
	if strings.TrimSpace(userID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendAuditLocked(userID, eventType, resource, "success", details, "System")
}

func (s *Service) findInsurancePolicy(userID, policyID string) (map[string]any, bool) {
	s.exp.mu.Lock()
	defer s.exp.mu.Unlock()
	if len(s.exp.insuranceData[userID]) == 0 {
		for _, legacy := range s.insurance[userID] {
			s.exp.insuranceData[userID] = append(s.exp.insuranceData[userID], map[string]any{
				"policy_id":         legacy.PolicyID,
				"category":          legacy.PolicyType,
				"insurer_name":      legacy.Insurer,
				"sum_assured":       legacy.SumAssuredPaise,
				"premium_amount":    legacy.PremiumPaise,
				"next_premium_date": legacy.NextDueDate,
				"verified":          true,
			})
		}
	}
	for _, item := range s.exp.insuranceData[userID] {
		if expString(item, "policy_id") == policyID {
			copyItem := map[string]any{}
			for k, v := range item {
				copyItem[k] = v
			}
			return copyItem, true
		}
	}
	return nil, false
}

func (s *Service) findLoan(userID, loanID string) (LoanRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, loan := range s.loans[userID] {
		if loan.LoanID == loanID {
			return loan, true
		}
	}
	return LoanRecord{}, false
}

// toDebtLoan maps a service LoanRecord into the pure debt.Loan input.
func toDebtLoan(l LoanRecord, floating bool) debt.Loan {
	return debt.Loan{
		ID:             l.LoanID,
		PrincipalPaise: l.OutstandingPaise,
		AnnualRatePct:  l.InterestRate,
		Months:         l.RemainingMonths,
		EMIPaise:       l.EMIPaise,
		Floating:       floating,
		LoanType:       l.LoanType,
	}
}

// inferFloating gives a reasonable floating/fixed default from the loan type when
// the client doesn't specify. Indian home/education/car loans are predominantly
// floating; personal/business/gold are typically fixed. The client can override.
func inferFloating(loanType string) bool {
	switch strings.ToLower(strings.TrimSpace(loanType)) {
	case "home", "education", "car", "auto", "vehicle":
		return true
	default:
		return false
	}
}

// parseCards builds credit.Card inputs from the request body. It accepts a "cards"
// array of objects; each may carry a statement_date (RFC3339) or statement_in_days
// offset. Returns nil when no cards are supplied (handler then reports zeros — a
// computed empty result, never a fake constant).
func parseCards(body map[string]any) []credit.Card {
	raw, ok := body["cards"].([]any)
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	cards := make([]credit.Card, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		stmt := now
		if s := expString(m, "statement_date"); s != "" {
			if parsed, err := time.Parse(time.RFC3339, s); err == nil {
				stmt = parsed
			}
		} else if d := expInt64(m, "statement_in_days"); d != 0 {
			stmt = now.Add(time.Duration(d) * 24 * time.Hour)
		}
		cards = append(cards, credit.Card{
			ID:               expString(m, "id"),
			OutstandingPaise: expInt64(m, "outstanding_paise"),
			LimitPaise:       expInt64(m, "limit_paise"),
			StatementDate:    stmt,
		})
	}
	return cards
}

// parseBNPLObligations builds credit.Obligation inputs and computes an effective
// APR for each when principal/total-repayable/tenure are supplied. Only entries
// that classify as BNPL (by MCC or provider registry) are included.
func parseBNPLObligations(body map[string]any, p config.CreditParams) ([]credit.Obligation, []float64) {
	raw, ok := body["obligations"].([]any)
	if !ok {
		return nil, nil
	}
	now := time.Now().UTC()
	obligations := make([]credit.Obligation, 0, len(raw))
	aprs := make([]float64, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		provider := expString(m, "provider")
		mcc := expString(m, "mcc")
		if !credit.IsBNPLObligation(mcc, provider, p) {
			continue
		}
		due := now
		if s := expString(m, "due_date"); s != "" {
			if parsed, err := time.Parse(time.RFC3339, s); err == nil {
				due = parsed
			}
		} else if d := expInt64(m, "due_in_days"); d != 0 {
			due = now.Add(time.Duration(d) * 24 * time.Hour)
		}
		obligations = append(obligations, credit.Obligation{
			Provider:       provider,
			DueAmountPaise: expInt64(m, "due_amount_paise"),
			DueDate:        due,
		})
		aprs = append(aprs, credit.EffectiveAPR(
			expInt64(m, "total_repayable_paise"),
			expInt64(m, "principal_paise"),
			int(expInt64(m, "tenure_days")),
		))
	}
	return obligations, aprs
}

func (s *Service) appendBlockchainRecordLocked(userID, recordType, referenceID string, payload any) map[string]any {
	if strings.TrimSpace(userID) == "" {
		userID = "internal"
	}
	txHash := expHash(fmt.Sprintf("%s:%s:%d", recordType, referenceID, time.Now().UnixNano()))
	blockNumber := time.Now().Unix()
	item := map[string]any{
		"id":           "bc-" + randomHex(8),
		"record_type":  recordType,
		"reference_id": referenceID,
		"payload_hash": expHash(fmt.Sprintf("%v", payload)),
		"merkle_root":  s.ledger.MerkleRoot(userID),
		"block_number": blockNumber,
		"tx_hash":      txHash,
		"created_at":   time.Now().UTC(),
	}
	s.exp.blockchainData[userID] = append(s.exp.blockchainData[userID], item)
	_, _ = s.ledger.WriteEvent(context.TODO(), userID, "blockchain_record", recordType, item)
	return item
}

func expString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func expInt64(data map[string]any, key string) int64 {
	if data == nil {
		return 0
	}
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func expFloat(data map[string]any, key string) float64 {
	if data == nil {
		return 0
	}
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func bearerTokenFromExpandedHeaders(headers map[string]string) string {
	for key, value := range headers {
		if !strings.EqualFold(strings.TrimSpace(key), "Authorization") {
			continue
		}

		header := strings.TrimSpace(value)
		if header == "" || !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			return ""
		}

		return strings.TrimSpace(header[7:])
	}

	return ""
}

func expHash(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}
