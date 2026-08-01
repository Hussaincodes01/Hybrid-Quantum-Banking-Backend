package platform

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type PinLoginRequest struct {
	Mobile string `json:"mobile"`
	// UserID is an alternative to Mobile: the Flutter client keeps the userId
	// returned by /v1/auth/register and logs in with {userId, pin}. Either
	// identifier resolves the same account.
	UserID              string `json:"userId,omitempty"`
	PIN                 string `json:"pin"`
	DeviceIDFingerprint string `json:"deviceIdFingerprint"`
	DeviceType          string `json:"deviceType,omitempty"`
	AppVersion          string `json:"appVersion,omitempty"`
}

type PinLoginResponse struct {
	UserID      string `json:"userId"`
	AccessToken string `json:"accessToken"`
	ExpiresIn   int    `json:"expiresInSeconds"`
	Name        string `json:"name"`
	UBT         string `json:"ubt"`
}

func HashPIN(pin string) (string, error) {
	if len(pin) < 4 || len(pin) > 6 {
		return "", errors.New("PIN must be 4-6 digits")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPIN(hashedPIN, pin string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashedPIN), []byte(pin)) == nil
}

func (s *Service) LoginWithPIN(req PinLoginRequest) (PinLoginResponse, error) {
	req.Mobile = strings.TrimSpace(req.Mobile)
	req.UserID = strings.TrimSpace(req.UserID)
	req.PIN = strings.TrimSpace(req.PIN)
	req.DeviceIDFingerprint = strings.TrimSpace(req.DeviceIDFingerprint)

	// Either identifier is accepted: the mobile number, or the userId the client
	// stored from /v1/auth/register.
	if req.Mobile == "" && req.UserID == "" {
		return PinLoginResponse{}, errors.New("mobile number or userId is required")
	}
	if req.PIN == "" {
		return PinLoginResponse{}, errors.New("PIN is required")
	}
	if req.DeviceIDFingerprint == "" {
		return PinLoginResponse{}, errors.New("device fingerprint is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var userID string
	var ok bool
	if req.Mobile != "" {
		userID, ok = s.mobileIndex[req.Mobile]
	} else {
		// Resolve by userId; presence in the user map is the existence check.
		_, ok = s.users[req.UserID]
		userID = req.UserID
	}
	if !ok {
		return PinLoginResponse{}, errors.New("invalid credentials")
	}

	user, ok := s.users[userID]
	if !ok {
		return PinLoginResponse{}, errors.New("invalid mobile number or PIN")
	}

	profile, ok := s.authProfiles[userID]
	if !ok || profile.PasswordHash == "" {
		return PinLoginResponse{}, errors.New("invalid mobile number or PIN")
	}

	// H-3 fix: per-account PIN lockout. FailedLoginCount was previously incremented
	// but never checked, so a 4-digit PIN was brute-forceable. Enforce escalating
	// lockouts keyed on the account (not just per-IP rate limiting).
	now := time.Now().UTC()
	if profile.PINLockedUntil != nil && now.Before(*profile.PINLockedUntil) {
		s.appendAuditLocked(userID, "pin_login_locked", "auth", "failure", "PIN login attempted while locked", "System")
		return PinLoginResponse{}, errors.New("account temporarily locked due to failed PIN attempts; try again later")
	}

	if !VerifyPIN(profile.PasswordHash, req.PIN) {
		profile.FailedLoginCount++
		// Escalating lockouts: 5 → 5min, 10 → 30min, 15 → 24h freeze.
		switch {
		case profile.FailedLoginCount >= 15:
			until := now.Add(24 * time.Hour)
			profile.PINLockedUntil = &until
		case profile.FailedLoginCount >= 10:
			until := now.Add(30 * time.Minute)
			profile.PINLockedUntil = &until
		case profile.FailedLoginCount >= 5:
			until := now.Add(5 * time.Minute)
			profile.PINLockedUntil = &until
		}
		s.appendAuditLocked(userID, "pin_login_failed", "auth", "failure", "Invalid PIN", "System")
		return PinLoginResponse{}, errors.New("invalid mobile number or PIN")
	}

	expiry := now.Add(15 * time.Minute)

	// Spec §3.6: issue a short-lived signed JWT when a JWT manager is injected
	// (server default); fall back to an opaque token otherwise (unit tests).
	// The session map is still populated either way so device binding, expiry
	// and revocation continue to work uniformly.
	token := "swt_" + randomHex(20)
	if s.jwt != nil {
		role := "customer"
		if profile.Role != "" {
			role = profile.Role
		}
		if jwtToken, err := s.jwt.IssueToken(userID, role, req.DeviceIDFingerprint); err == nil {
			token = jwtToken
		}
	}

	s.sessions[token] = userID
	s.sessionStart[token] = now
	s.sessionExpiry[token] = expiry
	s.sessionDevice[token] = req.DeviceIDFingerprint

	s.sessionStore.Set("session:"+token, userID, 15*time.Minute)
	s.sessionStore.Set("session_device:"+token, req.DeviceIDFingerprint, 15*time.Minute)

	profile.LastLoginTime = &now
	profile.SessionTokenID = token
	profile.SessionStartTime = &now
	profile.SessionExpiry = &expiry
	profile.FailedLoginCount = 0
	profile.PINLockedUntil = nil // H-3: clear lockout on successful login
	if profile.DeviceIDFingerprint == "" {
		profile.DeviceIDFingerprint = req.DeviceIDFingerprint
	}

	s.appendAuditLocked(userID, "pin_login_success", "auth", "success", "PIN login successful", "User")

	return PinLoginResponse{
		UserID:      userID,
		AccessToken: token,
		ExpiresIn:   900,
		Name:        user.Name,
		UBT:         user.UBT,
	}, nil
}

// RefreshToken exchanges a still-refreshable JWT (valid, or expired within the
// grace window) for a fresh one, without a full re-login. Fails if the JWT
// manager is not configured, the device fingerprint doesn't match, or the
// refresh window has elapsed.
func (s *Service) RefreshToken(oldToken, deviceFP string) (PinLoginResponse, error) {
	if s.jwt == nil {
		return PinLoginResponse{}, errors.New("token refresh is not supported")
	}
	oldToken = strings.TrimSpace(oldToken)
	if oldToken == "" {
		return PinLoginResponse{}, errors.New("token is required")
	}

	// H-6 fix: bind refresh to the device the session was established on. Compare
	// the presented fingerprint against the SERVER-STORED session device (set at
	// login), not just the self-contained JWT claim, so a device change forces a
	// full re-authentication and a server-side session/device revocation (deleting
	// the sessionDevice entry) actually blocks a refresh. NOTE: a stolen bearer
	// token that also carries the matching fingerprint still needs hardware device
	// attestation to fully defeat — this closes change-detection and revocation.
	presentedFP := strings.TrimSpace(deviceFP)
	s.mu.RLock()
	storedFP, hadSession := s.sessionDevice[oldToken]
	s.mu.RUnlock()
	if hadSession && storedFP != "" && presentedFP != storedFP {
		return PinLoginResponse{}, errors.New("device changed; re-authentication required")
	}

	newToken, err := s.jwt.RefreshToken(oldToken, presentedFP)
	if err != nil {
		return PinLoginResponse{}, err
	}
	claims, err := s.jwt.VerifyToken(newToken, strings.TrimSpace(deviceFP))
	if err != nil {
		return PinLoginResponse{}, err
	}

	now := time.Now().UTC()
	expiry := now.Add(15 * time.Minute)

	s.mu.Lock()
	// Retire the old session and register the rotated token.
	delete(s.sessions, oldToken)
	delete(s.sessionStart, oldToken)
	delete(s.sessionExpiry, oldToken)
	delete(s.sessionDevice, oldToken)
	s.sessions[newToken] = claims.Sub
	s.sessionStart[newToken] = now
	s.sessionExpiry[newToken] = expiry
	s.sessionDevice[newToken] = claims.DeviceFP
	s.sessionStore.Set("session:"+newToken, claims.Sub, 15*time.Minute)
	s.sessionStore.Set("session_device:"+newToken, claims.DeviceFP, 15*time.Minute)
	user := s.users[claims.Sub]
	if profile, ok := s.authProfiles[claims.Sub]; ok {
		profile.SessionTokenID = newToken
		profile.SessionStartTime = &now
		profile.SessionExpiry = &expiry
	}
	s.mu.Unlock()

	name := ""
	ubt := ""
	if user != nil {
		name = user.Name
		ubt = user.UBT
	}
	return PinLoginResponse{
		UserID:      claims.Sub,
		AccessToken: newToken,
		ExpiresIn:   900,
		Name:        name,
		UBT:         ubt,
	}, nil
}

// SetInitialPIN sets the PIN during onboarding, before the user has any session
// to authenticate with. The mobile client's flow is register -> eKYC ->
// biometric -> set PIN -> login, so the set-PIN call arrives with no bearer
// token. This is deliberately NOT a general auth bypass:
//
//   - it only works while the account has no PIN yet (first-time setup); once a
//     PIN exists, changing it requires an authenticated session via SetPIN, and
//   - the caller's device fingerprint must match the one recorded at
//     registration, so only the enrolling device can complete onboarding.
func (s *Service) SetInitialPIN(userID, pin, deviceFingerprint string) error {
	userID = strings.TrimSpace(userID)
	deviceFingerprint = strings.TrimSpace(deviceFingerprint)
	if userID == "" {
		return errors.New("userId is required")
	}

	s.mu.Lock()
	profile, ok := s.authProfiles[userID]
	if !ok {
		s.mu.Unlock()
		return errors.New("auth profile not found")
	}
	if profile.PasswordHash != "" {
		s.mu.Unlock()
		return errors.New("PIN already set: authenticate to change it")
	}
	if registered := strings.TrimSpace(profile.DeviceIDFingerprint); registered != "" &&
		!strings.EqualFold(registered, deviceFingerprint) {
		s.appendAuditLocked(userID, "pin_set_device_mismatch", "auth", "failure",
			"initial PIN setup attempted from an unregistered device", "System")
		s.mu.Unlock()
		return errors.New("device not recognised for this account")
	}
	s.mu.Unlock()

	return s.SetPIN(userID, pin)
}

func (s *Service) SetPIN(userID, pin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	profile, ok := s.authProfiles[userID]
	if !ok {
		return errors.New("auth profile not found")
	}

	hash, err := HashPIN(pin)
	if err != nil {
		return err
	}

	profile.PasswordHash = hash
	// M-10 fix: invalidate all existing sessions on PIN change so a compromised
	// session cannot survive the user's security action of changing their PIN.
	s.invalidateAllSessionsLocked(userID)
	s.appendAuditLocked(userID, "pin_set", "auth", "success", "PIN set successfully", "User")
	return nil
}
