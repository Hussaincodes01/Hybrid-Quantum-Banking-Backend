package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const simBindingChallengeTTL = 10 * time.Minute

type SIMBindingRecord struct {
	UserID        string     `json:"userId"`
	MobileNumber  string     `json:"mobileNumber"`
	DeviceID      string     `json:"deviceId"`
	SIMHash       string     `json:"simHash"`
	UBT           string     `json:"ubt"`
	Verified      bool       `json:"verified"`
	VerifiedAt    time.Time  `json:"verifiedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastCheckedAt time.Time  `json:"lastCheckedAt"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
}

type SIMBindingChallenge struct {
	ChallengeID  string `json:"challengeId"`
	UserID       string `json:"userId"`
	MobileNumber string `json:"mobileNumber"`
	DeviceID     string `json:"deviceId"`
	// VNNCode is the proof the client must present back to VerifyBindingChallenge.
	// It MUST NOT be serialised to the client (json:"-") or the verification is
	// self-attesting — an attacker who receives the code could bind an arbitrary
	// SIM/device to any user. Deliver it out of band (silent SMS / push to the VNN).
	VNNCode   string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Verified  bool      `json:"-"`
}

type SIMBindingManager struct {
	mu         sync.RWMutex
	bindings   map[string]*SIMBindingRecord
	challenges map[string]*SIMBindingChallenge
}

func NewSIMBindingManager() *SIMBindingManager {
	return &SIMBindingManager{
		bindings:   make(map[string]*SIMBindingRecord),
		challenges: make(map[string]*SIMBindingChallenge),
	}
}

// CreateBindingChallenge generates a verification challenge for SIM binding.
// In production, the app sends a silent SMS to a VNN (Virtual Network Node),
// and the telecom network verifies the sender's mobile matches the SIM.
// Here we simulate the VNN code generation.
func (m *SIMBindingManager) CreateBindingChallenge(userID, mobileNumber, deviceID string) (*SIMBindingChallenge, error) {
	if userID == "" || mobileNumber == "" {
		return nil, errors.New("userID and mobileNumber are required")
	}

	challengeBytes := make([]byte, 16)
	if _, err := rand.Read(challengeBytes); err != nil {
		return nil, fmt.Errorf("generate challenge id: %w", err)
	}
	challengeID := base64.URLEncoding.EncodeToString(challengeBytes)

	vnnBytes := make([]byte, 6)
	if _, err := rand.Read(vnnBytes); err != nil {
		return nil, fmt.Errorf("generate vnn code: %w", err)
	}
	vnnCode := fmt.Sprintf("VNN-%s", hex.EncodeToString(vnnBytes))

	now := time.Now().UTC()
	ch := &SIMBindingChallenge{
		ChallengeID:  challengeID,
		UserID:       userID,
		MobileNumber: mobileNumber,
		DeviceID:     deviceID,
		VNNCode:      vnnCode,
		CreatedAt:    now,
		ExpiresAt:    now.Add(simBindingChallengeTTL),
	}

	m.mu.Lock()
	m.pruneExpiredChallengesLocked(now) // drop abandoned challenges so the map can't grow unbounded
	m.challenges[challengeID] = ch
	m.mu.Unlock()

	return ch, nil
}

// pruneExpiredChallengesLocked removes timed-out challenges. Verified/expired
// challenges are already deleted on the verify path; this sweeps the ones a
// client created but never completed. Caller must hold m.mu.
func (m *SIMBindingManager) pruneExpiredChallengesLocked(now time.Time) {
	for id, c := range m.challenges {
		if now.After(c.ExpiresAt) {
			delete(m.challenges, id)
		}
	}
}

// VerifyBindingChallenge completes the SIM binding verification.
// In production, the VNN confirms the SMS came from the SIM in the device.
// Here we accept the VNN code as proof and create the binding.
func (m *SIMBindingManager) VerifyBindingChallenge(challengeID, vnnCode, simICCID string) (*SIMBindingRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, ok := m.challenges[challengeID]
	if !ok {
		return nil, errors.New("SIM binding challenge not found")
	}

	if ch.Verified {
		return nil, errors.New("challenge already used")
	}

	if time.Now().UTC().After(ch.ExpiresAt) {
		delete(m.challenges, challengeID)
		return nil, errors.New("SIM binding challenge has expired")
	}

	// Constant-time compare: the VNN code is a secret proof, so avoid a timing oracle.
	if subtle.ConstantTimeCompare([]byte(vnnCode), []byte(ch.VNNCode)) != 1 {
		return nil, errors.New("VNN code mismatch")
	}

	// Hash the SIM ICCID for storage (never store raw ICCID)
	simHash := hashSIMICCID(simICCID)

	// Generate UBT (Unique Binding Token)
	ubtBytes := make([]byte, 20)
	if _, err := rand.Read(ubtBytes); err != nil {
		return nil, fmt.Errorf("generate ubt: %w", err)
	}
	ubt := "ubt_" + hex.EncodeToString(ubtBytes)

	now := time.Now().UTC()
	record := &SIMBindingRecord{
		UserID:        ch.UserID,
		MobileNumber:  ch.MobileNumber,
		DeviceID:      ch.DeviceID, // audit fix: bind to the actual device, not the user-id string
		SIMHash:       simHash,
		UBT:           ubt,
		Verified:      true,
		VerifiedAt:    now,
		CreatedAt:     now,
		LastCheckedAt: now,
	}

	// LIMITATION: one binding per user — keyed by UserID alone, so a new binding
	// on a second device overwrites the first. Multi-device SIM binding is not a
	// requirement today; if it becomes one, key by UserID+DeviceID here and in
	// the lookup helpers (VerifyUBT / ReverifySIM / RevokeBinding / GetBinding /
	// IsBound / DetectSIMSwap).
	m.bindings[ch.UserID] = record
	ch.Verified = true
	delete(m.challenges, challengeID)

	return record, nil
}

// VerifyUBT checks if the UBT is still valid for the given user.
// Per spec §3.2: "The UBT is verified on every app open and on every transaction"
func (m *SIMBindingManager) VerifyUBT(userID, ubt string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, ok := m.bindings[userID]
	if !ok {
		return false, errors.New("no SIM binding found for user")
	}

	if record.RevokedAt != nil {
		return false, errors.New("SIM binding has been revoked")
	}

	// Constant-time compare: the UBT is a bearer secret checked on every request.
	if subtle.ConstantTimeCompare([]byte(record.UBT), []byte(ubt)) != 1 {
		return false, errors.New("UBT mismatch")
	}

	return true, nil
}

// ReverifySIM checks if the SIM is still present by comparing the current SIM hash.
func (m *SIMBindingManager) ReverifySIM(userID, currentSIMHash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, ok := m.bindings[userID]
	if !ok {
		return false, errors.New("no SIM binding found for user")
	}

	if record.SIMHash != currentSIMHash {
		return false, errors.New("SIM mismatch — different SIM card detected")
	}

	record.LastCheckedAt = time.Now().UTC()
	return true, nil
}

// RevokeBinding revokes the SIM binding (used for SIM swap detection).
func (m *SIMBindingManager) RevokeBinding(userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, ok := m.bindings[userID]
	if !ok {
		return errors.New("no SIM binding found for user")
	}

	now := time.Now().UTC()
	record.RevokedAt = &now
	record.UBT = ""
	return nil
}

// GetBinding returns the SIM binding record for a user.
func (m *SIMBindingManager) GetBinding(userID string) (*SIMBindingRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, ok := m.bindings[userID]
	if !ok {
		return nil, errors.New("no SIM binding found for user")
	}

	// Return a copy: the stored *record is mutated in place under the lock by
	// ReverifySIM / RevokeBinding, so handing out the live pointer would let
	// callers race those writers. (Mirrors ListRegistrations in webauthn.go.)
	cp := *record
	return &cp, nil
}

// IsBound checks if a user has a verified SIM binding.
func (m *SIMBindingManager) IsBound(userID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, ok := m.bindings[userID]
	return ok && record.Verified && record.RevokedAt == nil
}

// DetectSIMSwap checks if the SIM hash has changed since binding.
func (m *SIMBindingManager) DetectSIMSwap(userID, currentSIMHash string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, ok := m.bindings[userID]
	if !ok {
		return false
	}

	return record.SIMHash != currentSIMHash
}

// hashSIMICCID computes a keyed digest of the ICCID.
// M-15 fix: plain SHA-256 over a finite (19-20 digit) identifier space is
// reversible via a precomputed table. Use HMAC-SHA256 with a server-side pepper
// so a database leak cannot be reversed without the key.
func hashSIMICCID(iccid string) string {
	mac := hmac.New(sha256.New, simHashKey())
	mac.Write([]byte(iccid))
	return hex.EncodeToString(mac.Sum(nil))
}

// simHashKey returns the server-side pepper for SIM hashing. Prefers a dedicated
// FINIX_SIM_HASH_KEY, falling back to the mandatory FINIX_JWT_SECRET (a production
// server cannot boot without it), so there is always a real secret and never a
// hardcoded constant.
func simHashKey() []byte {
	if k := strings.TrimSpace(os.Getenv("FINIX_SIM_HASH_KEY")); k != "" {
		return []byte(k)
	}
	return []byte(strings.TrimSpace(os.Getenv("FINIX_JWT_SECRET")))
}

// HashSIMICCID is exported for use by the platform service.
func HashSIMICCID(iccid string) string {
	return hashSIMICCID(iccid)
}
