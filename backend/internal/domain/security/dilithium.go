package security

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cloudflare/circl/sign/dilithium/mode3"
)

const (
	dilithiumAlgorithm   = "CRYSTALS-Dilithium3"
	dilithiumKeyRotation = 24 * time.Hour
	dilithiumGracePeriod = 1 * time.Hour
)

type DilithiumKeypair struct {
	KeyID         string
	PublicKey     []byte
	PrivateKey    []byte
	Algorithm     string
	CreatedAt     time.Time
	RotatedAt     time.Time
	RotationActor string
}

type DilithiumSignature struct {
	KeyID     string    `json:"keyId"`
	Algorithm string    `json:"algorithm"`
	Signature string    `json:"signature"`
	SignedAt  time.Time `json:"signedAt"`
}

// DilithiumKeyInfo is the client-safe, serialisable view of the active keypair.
// It deliberately OMITS the private key: GetCurrentKeyInfo returns this type so a
// handler that JSON-encodes "key info" can never leak the signing secret. Only
// non-sensitive metadata (id, algorithm, public key, timestamps, rotation actor)
// is present.
type DilithiumKeyInfo struct {
	KeyID         string    `json:"keyId"`
	Algorithm     string    `json:"algorithm"`
	PublicKey     string    `json:"publicKey"` // base64-encoded, safe to publish
	CreatedAt     time.Time `json:"createdAt"`
	RotatedAt     time.Time `json:"rotatedAt"`
	RotationActor string    `json:"rotationActor,omitempty"`
}

type DilithiumManager struct {
	mu           sync.RWMutex
	current      *DilithiumKeypair
	previous     *DilithiumKeypair
	rotatedAt    time.Time
	nextRotation time.Time
}

func NewDilithiumManager() (*DilithiumManager, error) {
	pk, sk, err := mode3.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate dilithium keypair: %w", err)
	}

	keyIDBytes := make([]byte, 16)
	if _, err := rand.Read(keyIDBytes); err != nil {
		return nil, fmt.Errorf("generate key ID: %w", err)
	}
	keyID := "dilithium_" + base64.URLEncoding.EncodeToString(keyIDBytes)
	now := time.Now().UTC()

	pair := &DilithiumKeypair{
		KeyID:         keyID,
		PublicKey:     pk.Bytes(),
		PrivateKey:    sk.Bytes(),
		Algorithm:     dilithiumAlgorithm,
		CreatedAt:     now,
		RotatedAt:     now,
		RotationActor: "system",
	}

	return &DilithiumManager{
		current:      pair,
		rotatedAt:    now,
		nextRotation: now.Add(dilithiumKeyRotation),
	}, nil
}

func (d *DilithiumManager) Sign(data []byte) (*DilithiumSignature, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.current == nil {
		return nil, errors.New("dilithium manager not initialized")
	}

	sk := new(mode3.PrivateKey)
	if err := sk.UnmarshalBinary(d.current.PrivateKey); err != nil {
		return nil, fmt.Errorf("unmarshal private key: %w", err)
	}

	sig := make([]byte, mode3.SignatureSize)
	mode3.SignTo(sk, data, sig)

	return &DilithiumSignature{
		KeyID:     d.current.KeyID,
		Algorithm: d.current.Algorithm,
		Signature: base64.StdEncoding.EncodeToString(sig),
		SignedAt:  time.Now().UTC(),
	}, nil
}

func (d *DilithiumManager) Verify(sig *DilithiumSignature, data []byte) (bool, error) {
	if sig == nil {
		return false, errors.New("nil signature")
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil {
		return false, fmt.Errorf("decode signature: %w", err)
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	var pubKey *mode3.PublicKey
	if d.current != nil && d.current.KeyID == sig.KeyID {
		pubKey = new(mode3.PublicKey)
		if err := pubKey.UnmarshalBinary(d.current.PublicKey); err != nil {
			return false, fmt.Errorf("unmarshal current public key: %w", err)
		}
	} else if d.previous != nil && d.previous.KeyID == sig.KeyID {
		pubKey = new(mode3.PublicKey)
		if err := pubKey.UnmarshalBinary(d.previous.PublicKey); err != nil {
			return false, fmt.Errorf("unmarshal previous public key: %w", err)
		}
	} else {
		return false, errors.New("unknown key ID: signature was not created with a known key")
	}

	return mode3.Verify(pubKey, data, sigBytes), nil
}

// GetCurrentKeyInfo returns a redacted, client-safe view of the current keypair.
// The private key is never included — callers that only need public metadata
// (handlers, health probes) use this; signing reads d.current.PrivateKey directly.
func (d *DilithiumManager) GetCurrentKeyInfo() DilithiumKeyInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return DilithiumKeyInfo{
		KeyID:         d.current.KeyID,
		Algorithm:     d.current.Algorithm,
		PublicKey:     base64.StdEncoding.EncodeToString(d.current.PublicKey),
		CreatedAt:     d.current.CreatedAt,
		RotatedAt:     d.current.RotatedAt,
		RotationActor: d.current.RotationActor,
	}
}

func (d *DilithiumManager) GetPublicKeyB64() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return base64.StdEncoding.EncodeToString(d.current.PublicKey)
}

func (d *DilithiumManager) GetKeyID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.current.KeyID
}

func (d *DilithiumManager) RotateKey(actor string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rotateLocked(actor)
}

// rotateLocked performs a key rotation assuming d.mu is already held, so callers
// (RotateKey, MaybeRotate) never drop the lock mid-rotation — two concurrent
// callers can no longer rotate twice back-to-back and evict the genuine previous
// key from the grace window.
func (d *DilithiumManager) rotateLocked(actor string) error {
	pk, sk, err := mode3.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate new dilithium keypair: %w", err)
	}

	keyIDBytes := make([]byte, 16)
	if _, err := rand.Read(keyIDBytes); err != nil {
		return fmt.Errorf("generate key ID: %w", err)
	}
	keyID := "dilithium_" + base64.URLEncoding.EncodeToString(keyIDBytes)
	now := time.Now().UTC()

	if d.current != nil {
		old := *d.current
		d.previous = &old
	}

	d.current = &DilithiumKeypair{
		KeyID:         keyID,
		PublicKey:     pk.Bytes(),
		PrivateKey:    sk.Bytes(),
		Algorithm:     dilithiumAlgorithm,
		CreatedAt:     now,
		RotatedAt:     now,
		RotationActor: actor,
	}
	d.rotatedAt = now
	d.nextRotation = now.Add(dilithiumKeyRotation)

	return nil
}

func (d *DilithiumManager) MaybeRotate(actor string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if time.Now().UTC().After(d.nextRotation) {
		return d.rotateLocked(actor) == nil
	}
	return false
}

func (d *DilithiumManager) PrunePreviousKey() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.previous != nil && time.Since(d.rotatedAt) > dilithiumGracePeriod {
		d.previous = nil
	}
}

func (d *DilithiumManager) StartJanitor(stopCh <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.MaybeRotate("system-janitor")
				d.PrunePreviousKey()
			case <-stopCh:
				return
			}
		}
	}()
}
