package security

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cloudflare/circl/kem/kyber/kyber1024"
)

const (
	pqcSessionTTL          = 5 * time.Minute
	pqcKeyRotationInterval = 24 * time.Hour
	pqcKeyGracePeriod      = 1 * time.Hour
	pqcJanitorInterval     = 60 * time.Second
)

type PQCSession struct {
	SessionID    string
	SharedSecret []byte
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type PQCKeyInfo struct {
	KeyID         string    `json:"keyId"`
	Algorithm     string    `json:"algorithm"`
	PublicKeyRef  string    `json:"publicKeyRef"`
	KeyCreatedAt  time.Time `json:"keyCreatedAt"`
	KeyRotatedAt  time.Time `json:"keyRotatedAt"`
	RotationActor string    `json:"rotationActor"`
}

type PQCManager struct {
	mu            sync.RWMutex
	ServerKeypair *Keypair
	keyInfo       *PQCKeyInfo
	previousKey   *Keypair
	previousInfo  *PQCKeyInfo
	sessions      map[string]*PQCSession
	rotatedAt     time.Time
	nextRotation  time.Time
	stopJanitor   chan struct{}
}

type Keypair struct {
	PublicKey  [kyber1024.PublicKeySize]byte
	PrivateKey [kyber1024.PrivateKeySize]byte
}

func NewPQCManager() (*PQCManager, error) {
	pk, sk, err := kyber1024.GenerateKeyPair(rand.Reader)
	if err != nil {
		return nil, err
	}

	kp := &Keypair{}
	pk.Pack(kp.PublicKey[:])
	sk.Pack(kp.PrivateKey[:])

	keyIDBytes := make([]byte, 16)
	if _, err := rand.Read(keyIDBytes); err != nil {
		return nil, fmt.Errorf("generate key id: %w", err)
	}
	keyID := "kyber_" + base64.URLEncoding.EncodeToString(keyIDBytes)
	now := time.Now().UTC()

	mgr := &PQCManager{
		ServerKeypair: kp,
		keyInfo: &PQCKeyInfo{
			KeyID:         keyID,
			Algorithm:     "CRYSTALS-Kyber-1024",
			PublicKeyRef:  base64.StdEncoding.EncodeToString(kp.PublicKey[:]),
			KeyCreatedAt:  now,
			KeyRotatedAt:  now,
			RotationActor: "system",
		},
		sessions:     make(map[string]*PQCSession),
		rotatedAt:    now,
		nextRotation: now.Add(pqcKeyRotationInterval),
		stopJanitor:  make(chan struct{}),
	}

	mgr.startJanitor()
	return mgr, nil
}

func (m *PQCManager) GetPublicKeyB64() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return base64.StdEncoding.EncodeToString(m.ServerKeypair.PublicKey[:])
}

func (m *PQCManager) GetKeyInfo() PQCKeyInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.keyInfo
}

// selectPrivateKeyLocked returns a copy of the private-key bytes for keyID.
// Caller must hold m.mu (read or write).
func (m *PQCManager) selectPrivateKeyLocked(keyID string) ([]byte, error) {
	if m.ServerKeypair != nil && (keyID == "" || (m.keyInfo != nil && keyID == m.keyInfo.KeyID)) {
		return append([]byte(nil), m.ServerKeypair.PrivateKey[:]...), nil
	}
	if m.previousKey != nil && m.previousInfo != nil && keyID == m.previousInfo.KeyID {
		return append([]byte(nil), m.previousKey.PrivateKey[:]...), nil
	}
	return nil, errors.New("unknown or expired PQC key id")
}

// Decapsulate unwraps a ciphertext with the current server key. Prefer
// DecapsulateWithKey when the client knows which key it encapsulated to.
func (m *PQCManager) Decapsulate(ciphertextB64 string) (string, []byte, error) {
	return m.DecapsulateWithKey(ciphertextB64, "")
}

// DecapsulateWithKey unwraps a ciphertext with the keypair identified by keyID.
// An empty keyID (or the current key's id) uses the current key; the previous
// key is accepted while it is still within the rotation grace window, so a client
// that encapsulated to the just-rotated public key still gets the right shared
// secret. An unknown/expired keyID is rejected rather than silently returning a
// garbage secret.
func (m *PQCManager) DecapsulateWithKey(ciphertextB64, keyID string) (string, []byte, error) {
	ct, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", nil, errors.New("invalid base64 ciphertext")
	}

	if len(ct) != kyber1024.CiphertextSize {
		return "", nil, errors.New("invalid ciphertext size")
	}

	var ctArr [kyber1024.CiphertextSize]byte
	copy(ctArr[:], ct)

	m.mu.RLock()
	privBytes, err := m.selectPrivateKeyLocked(keyID)
	m.mu.RUnlock()
	if err != nil {
		return "", nil, err
	}
	sk := new(kyber1024.PrivateKey)
	sk.Unpack(privBytes)

	sharedSecret := make([]byte, kyber1024.SharedKeySize)
	sk.DecapsulateTo(sharedSecret, ctArr[:])

	sessionBytes := make([]byte, 16)
	if _, err := rand.Read(sessionBytes); err != nil {
		return "", nil, fmt.Errorf("generate session id: %w", err)
	}
	sessionID := base64.URLEncoding.EncodeToString(sessionBytes)

	now := time.Now().UTC()
	session := &PQCSession{
		SessionID:    sessionID,
		SharedSecret: sharedSecret,
		CreatedAt:    now,
		ExpiresAt:    now.Add(pqcSessionTTL),
	}

	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	return sessionID, sharedSecret, nil
}

func (m *PQCManager) ValidatePQCAuth(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		m.mu.Lock()
		delete(m.sessions, sessionID)
		m.mu.Unlock()
		return false
	}

	return true
}

func (m *PQCManager) DeriveSessionKey(sessionID string) ([]byte, error) {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return nil, errors.New("PQC session not found")
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		m.mu.Lock()
		delete(m.sessions, sessionID)
		m.mu.Unlock()
		return nil, errors.New("PQC session has expired")
	}

	return DeriveKeyFromSharedSecret(session.SharedSecret)
}

func (m *PQCManager) Encrypt(sessionID string, plaintext []byte) (*EncryptedPayload, error) {
	key, err := m.DeriveSessionKey(sessionID)
	if err != nil {
		return nil, err
	}

	payload, err := EncryptAESGCM(key, plaintext)
	if err != nil {
		return nil, err
	}

	return &payload, nil
}

func (m *PQCManager) Decrypt(sessionID string, payload EncryptedPayload) ([]byte, error) {
	key, err := m.DeriveSessionKey(sessionID)
	if err != nil {
		return nil, err
	}

	return DecryptAESGCM(key, payload)
}

func (m *PQCManager) RotateKey(actor string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rotateLocked(actor)
}

// rotateLocked performs a key rotation assuming m.mu is already held, so callers
// (RotateKey, MaybeRotate) never drop the lock mid-rotation — two concurrent
// callers can no longer rotate twice back-to-back and evict the genuine previous
// key from the grace window.
func (m *PQCManager) rotateLocked(actor string) error {
	pk, sk, err := kyber1024.GenerateKeyPair(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate new kyber keypair: %w", err)
	}

	newKp := &Keypair{}
	pk.Pack(newKp.PublicKey[:])
	sk.Pack(newKp.PrivateKey[:])

	keyIDBytes := make([]byte, 16)
	if _, err := rand.Read(keyIDBytes); err != nil {
		return fmt.Errorf("generate key id: %w", err)
	}
	keyID := "kyber_" + base64.URLEncoding.EncodeToString(keyIDBytes)
	now := time.Now().UTC()

	if m.ServerKeypair != nil {
		old := *m.ServerKeypair
		m.previousKey = &old
		if m.keyInfo != nil {
			oldInfo := *m.keyInfo
			m.previousInfo = &oldInfo
		}
	}

	m.ServerKeypair = newKp
	m.keyInfo = &PQCKeyInfo{
		KeyID:         keyID,
		Algorithm:     "CRYSTALS-Kyber-1024",
		PublicKeyRef:  base64.StdEncoding.EncodeToString(newKp.PublicKey[:]),
		KeyCreatedAt:  now,
		KeyRotatedAt:  now,
		RotationActor: actor,
	}
	m.rotatedAt = now
	m.nextRotation = now.Add(pqcKeyRotationInterval)

	slog.Info("PQC Kyber keypair rotated", "keyId", keyID, "actor", actor)
	return nil
}

func (m *PQCManager) MaybeRotate(actor string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if time.Now().UTC().After(m.nextRotation) {
		return m.rotateLocked(actor) == nil
	}
	return false
}

func (m *PQCManager) startJanitor() {
	ticker := time.NewTicker(pqcJanitorInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.cleanupExpiredSessions()
				m.MaybeRotate("system-janitor")
				m.prunePreviousKey()
			case <-m.stopJanitor:
				return
			}
		}
	}()
}

func (m *PQCManager) StopJanitor() {
	select {
	case <-m.stopJanitor:
	default:
		close(m.stopJanitor)
	}
}

func (m *PQCManager) cleanupExpiredSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for id, session := range m.sessions {
		if now.After(session.ExpiresAt) {
			delete(m.sessions, id)
		}
	}
}

func (m *PQCManager) prunePreviousKey() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.previousKey != nil && time.Since(m.rotatedAt) > pqcKeyGracePeriod {
		m.previousKey = nil
		m.previousInfo = nil
	}
}

func (m *PQCManager) SessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
