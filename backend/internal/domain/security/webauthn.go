package security

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"math/big"
	"sync"
	"time"
)

var (
	ErrChallengeNotFound       = errors.New("passkey challenge not found")
	ErrChallengeExpired        = errors.New("passkey challenge has expired")
	ErrChallengeUsed           = errors.New("passkey challenge already used")
	ErrInvalidSignature        = errors.New("invalid passkey signature")
	ErrKeyIDNotFound           = errors.New("passkey key ID not found for user")
	ErrRegistrationExists      = errors.New("passkey registration already exists for this key ID")
	ErrNoRegistrations         = errors.New("no passkey registrations found for user")
	ErrChallengeActionMismatch = errors.New("passkey challenge was issued for a different action")
)

const (
	challengeLength = 32
	challengeTTL    = 5 * time.Minute
)

type PasskeyRegistration struct {
	KeyID      string    `json:"keyId"`
	UserID     string    `json:"userId"`
	PublicKey  []byte    `json:"publicKey"`
	SignCount  uint32    `json:"signCount"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	DeviceName string    `json:"deviceName"`
}

type PasskeyChallenge struct {
	ChallengeID string    `json:"challengeId"`
	Challenge   []byte    `json:"challenge"`
	UserID      string    `json:"userId"`
	Action      string    `json:"action,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Used        bool      `json:"-"`
}

type PasskeyManager struct {
	mu            sync.RWMutex
	registrations map[string][]PasskeyRegistration
	challenges    map[string]*PasskeyChallenge
}

func NewPasskeyManager() *PasskeyManager {
	return &PasskeyManager{
		registrations: make(map[string][]PasskeyRegistration),
		challenges:    make(map[string]*PasskeyChallenge),
	}
}

func (pm *PasskeyManager) CreateChallenge(userID string) (*PasskeyChallenge, error) {
	return pm.CreateChallengeForAction(userID, "")
}

// CreateChallengeForAction creates a challenge bound to a specific action (M-3).
// The action label (e.g. "transaction_override:txn-123") is folded into the
// signed digest, so a signature captured for one operation cannot be replayed to
// authorize a different one within the challenge TTL. Passing an empty action
// preserves the legacy un-bound behaviour.
func (pm *PasskeyManager) CreateChallengeForAction(userID, action string) (*PasskeyChallenge, error) {
	challengeBytes := make([]byte, challengeLength)
	if _, err := rand.Read(challengeBytes); err != nil {
		return nil, err
	}

	challengeIDBytes := make([]byte, 16)
	if _, err := rand.Read(challengeIDBytes); err != nil {
		return nil, err
	}

	now := time.Now()
	ch := &PasskeyChallenge{
		ChallengeID: base64.URLEncoding.EncodeToString(challengeIDBytes),
		Challenge:   challengeBytes,
		UserID:      userID,
		Action:      action,
		CreatedAt:   now,
		ExpiresAt:   now.Add(challengeTTL),
	}

	pm.mu.Lock()
	pm.pruneChallengesLocked(now) // sweep timed-out challenges so the map can't grow unbounded
	pm.challenges[ch.ChallengeID] = ch
	pm.mu.Unlock()

	return ch, nil
}

// pruneChallengesLocked deletes challenges past their expiry. Used-but-unexpired
// challenges are retained so a replay still gets ErrChallengeUsed within the TTL.
// Caller must hold pm.mu.
func (pm *PasskeyManager) pruneChallengesLocked(now time.Time) {
	for id, c := range pm.challenges {
		if now.After(c.ExpiresAt) {
			delete(pm.challenges, id)
		}
	}
}

func (pm *PasskeyManager) VerifyChallenge(userID, challengeID string, signature []byte, keyID string) (bool, error) {
	return pm.VerifyChallengeForAction(userID, challengeID, signature, keyID, "")
}

// VerifyChallengeForAction verifies a signature against a challenge and, when
// expectedAction is non-empty, requires the challenge to have been created for
// that same action (M-3). The action is mixed into the signed digest so a
// mismatch fails signature verification rather than silently authorizing the
// wrong operation.
func (pm *PasskeyManager) VerifyChallengeForAction(userID, challengeID string, signature []byte, keyID, expectedAction string) (bool, error) {
	// Read phase: validate the challenge and snapshot the public key + challenge
	// bytes. We deliberately do NOT mark the challenge Used yet (N22): a malformed
	// attempt or unknown keyID must not permanently consume a still-valid
	// challenge. We also copy the pubkey rather than hold a pointer into the
	// registrations slice, which a concurrent Register (append reallocates) or
	// RemoveRegistration (element shift) could invalidate.
	pm.mu.RLock()
	ch, ok := pm.challenges[challengeID]
	if !ok {
		pm.mu.RUnlock()
		return false, ErrChallengeNotFound
	}
	if ch.Used {
		pm.mu.RUnlock()
		return false, ErrChallengeUsed
	}
	if time.Now().After(ch.ExpiresAt) {
		pm.mu.RUnlock()
		return false, ErrChallengeExpired
	}
	if ch.UserID != userID {
		pm.mu.RUnlock()
		return false, ErrInvalidSignature
	}
	// M-3: the challenge must have been minted for the action being authorized.
	if expectedAction != "" && ch.Action != expectedAction {
		pm.mu.RUnlock()
		return false, ErrChallengeActionMismatch
	}
	var pubKeyDER []byte
	found := false
	for i := range pm.registrations[userID] {
		if pm.registrations[userID][i].KeyID == keyID {
			pubKeyDER = append([]byte(nil), pm.registrations[userID][i].PublicKey...)
			found = true
			break
		}
	}
	challengeBytes := append([]byte(nil), ch.Challenge...)
	boundAction := ch.Action
	pm.mu.RUnlock()

	if !found {
		return false, ErrKeyIDNotFound
	}

	pubKey, err := parseECDSAPublicKey(pubKeyDER)
	if err != nil {
		return false, err
	}
	// Bind the action into the digest so the signature only validates for the
	// operation the challenge was issued for. Empty action => legacy digest.
	signed := challengeBytes
	if boundAction != "" {
		signed = append(append([]byte(nil), challengeBytes...), []byte("\x00"+boundAction)...)
	}
	digest := sha256.Sum256(signed)
	if !ecdsa.VerifyASN1(pubKey, digest[:], signature) {
		// Invalid signature — leave the challenge unconsumed for a legitimate retry.
		return false, nil
	}

	// Commit phase: consume the challenge and bump the counter under a single
	// write lock. Re-check Used to keep the challenge strictly single-use under
	// concurrent verifies, and re-look up the registration by keyID so we never
	// mutate through a pointer captured before the unlock.
	pm.mu.Lock()
	defer pm.mu.Unlock()
	cur, ok := pm.challenges[challengeID]
	if !ok || cur.Used {
		return false, ErrChallengeUsed
	}
	cur.Used = true
	for i := range pm.registrations[userID] {
		if pm.registrations[userID][i].KeyID == keyID {
			pm.registrations[userID][i].SignCount++
			pm.registrations[userID][i].LastUsedAt = time.Now()
			break
		}
	}
	return true, nil
}

func (pm *PasskeyManager) Register(userID, keyID, deviceName string, publicKey []byte) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, reg := range pm.registrations[userID] {
		if reg.KeyID == keyID {
			return ErrRegistrationExists
		}
	}

	pm.registrations[userID] = append(pm.registrations[userID], PasskeyRegistration{
		KeyID:      keyID,
		UserID:     userID,
		PublicKey:  publicKey,
		SignCount:  0,
		CreatedAt:  time.Now(),
		LastUsedAt: time.Time{},
		DeviceName: deviceName,
	})

	return nil
}

func (pm *PasskeyManager) ListRegistrations(userID string) []PasskeyRegistration {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	regs := pm.registrations[userID]
	if regs == nil {
		return nil
	}

	result := make([]PasskeyRegistration, len(regs))
	copy(result, regs)
	return result
}

func (pm *PasskeyManager) RemoveRegistration(userID, keyID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	regs := pm.registrations[userID]
	for i, reg := range regs {
		if reg.KeyID == keyID {
			pm.registrations[userID] = append(regs[:i], regs[i+1:]...)
			return nil
		}
	}

	return ErrKeyIDNotFound
}

func (pm *PasskeyManager) HasRegistration(userID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return len(pm.registrations[userID]) > 0
}

func parseECDSAPublicKey(der []byte) (*ecdsa.PublicKey, error) {
	// crypto/ecdh replaces the deprecated elliptic.Unmarshal (Go 1.21) and also
	// validates the point is a canonical, on-curve P-256 point.
	key, err := ecdh.P256().NewPublicKey(der)
	if err != nil {
		return nil, errors.New("invalid elliptic curve public key encoding")
	}
	raw := key.Bytes() // uncompressed SEC1: 0x04 || X(32) || Y(32)
	if len(raw) != 65 || raw[0] != 0x04 {
		return nil, errors.New("unsupported public key format")
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(raw[1:33]),
		Y:     new(big.Int).SetBytes(raw[33:]),
	}, nil
}

func MarshalECDSAPublicKey(pub *ecdsa.PublicKey) []byte {
	// Uncompressed SEC1 encoding via crypto/ecdh (replaces deprecated elliptic.Marshal).
	k, err := pub.ECDH()
	if err != nil {
		return nil
	}
	return k.Bytes()
}

func GeneratePasskeyKeyPair() (*ecdsa.PrivateKey, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	pubBytes := MarshalECDSAPublicKey(&privateKey.PublicKey)
	return privateKey, pubBytes, nil
}
