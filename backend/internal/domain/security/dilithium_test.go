package security

import (
	"crypto/rand"
	"testing"
	"time"
)

func getCurrentTime() time.Time {
	return time.Now().UTC()
}

func TestDilithiumSignAndVerify(t *testing.T) {
	dm, err := NewDilithiumManager()
	if err != nil {
		t.Fatalf("failed to create dilithium manager: %v", err)
	}

	message := []byte("test transaction data for dilithium signing")
	sig, err := dm.Sign(message)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	if sig.KeyID == "" {
		t.Fatal("expected non-empty key ID")
	}
	if sig.Algorithm != dilithiumAlgorithm {
		t.Fatalf("expected algorithm %s, got %s", dilithiumAlgorithm, sig.Algorithm)
	}
	if sig.Signature == "" {
		t.Fatal("expected non-empty signature")
	}

	valid, err := dm.Verify(sig, message)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if !valid {
		t.Fatal("expected signature to be valid")
	}
}

func TestDilithiumVerifyWrongData(t *testing.T) {
	dm, err := NewDilithiumManager()
	if err != nil {
		t.Fatalf("failed to create dilithium manager: %v", err)
	}

	message := []byte("original message")
	sig, err := dm.Sign(message)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	wrongMessage := []byte("tampered message")
	valid, _ := dm.Verify(sig, wrongMessage)
	if valid {
		t.Fatal("expected signature verification to fail for wrong data")
	}
}

func TestDilithiumKeyRotation(t *testing.T) {
	dm, err := NewDilithiumManager()
	if err != nil {
		t.Fatalf("failed to create dilithium manager: %v", err)
	}

	originalKeyID := dm.GetKeyID()
	message := []byte("test data before rotation")
	sig, err := dm.Sign(message)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	if err := dm.RotateKey("test-actor"); err != nil {
		t.Fatalf("rotate key failed: %v", err)
	}

	newKeyID := dm.GetKeyID()
	if newKeyID == originalKeyID {
		t.Fatal("expected new key ID after rotation")
	}

	keyInfo := dm.GetCurrentKeyInfo()
	if keyInfo.RotationActor != "test-actor" {
		t.Fatalf("expected rotation actor 'test-actor', got %s", keyInfo.RotationActor)
	}

	// Old signature should still verify (previous key is kept during grace period)
	valid, err := dm.Verify(sig, message)
	if err != nil {
		t.Fatalf("verify old signature failed: %v", err)
	}
	if !valid {
		t.Fatal("expected old signature to still verify after rotation (grace period)")
	}
}

func TestDilithiumUnknownKeyID(t *testing.T) {
	dm, err := NewDilithiumManager()
	if err != nil {
		t.Fatalf("failed to create dilithium manager: %v", err)
	}

	sig := &DilithiumSignature{
		KeyID:     "unknown_key_id",
		Algorithm: dilithiumAlgorithm,
		Signature: "dGhpcyBpcyBhIGZha2Ugc2lnbmF0dXJl",
	}

	valid, err := dm.Verify(sig, []byte("test data"))
	if err == nil {
		t.Fatal("expected error for unknown key ID")
	}
	if valid {
		t.Fatal("expected verification to fail for unknown key ID")
	}
}

func TestDilithiumRandomDataSigning(t *testing.T) {
	dm, err := NewDilithiumManager()
	if err != nil {
		t.Fatalf("failed to create dilithium manager: %v", err)
	}

	for i := 0; i < 5; i++ {
		data := make([]byte, 256)
		rand.Read(data)

		sig, err := dm.Sign(data)
		if err != nil {
			t.Fatalf("sign iteration %d failed: %v", i, err)
		}

		valid, err := dm.Verify(sig, data)
		if err != nil {
			t.Fatalf("verify iteration %d failed: %v", i, err)
		}
		if !valid {
			t.Fatalf("expected signature %d to be valid", i)
		}
	}
}

func TestPQCSessionExpiry(t *testing.T) {
	mgr, err := NewPQCManager()
	if err != nil {
		t.Fatalf("failed to create PQC manager: %v", err)
	}
	defer mgr.StopJanitor()

	// Create a fake session by manually inserting one
	sessionID := "test-session-expiry"
	mgr.mu.Lock()
	mgr.sessions[sessionID] = &PQCSession{
		SessionID:    sessionID,
		SharedSecret: []byte("test-secret"),
		CreatedAt:    getCurrentTime(),
		ExpiresAt:    getCurrentTime().Add(-1 * time.Second), // Already expired
	}
	mgr.mu.Unlock()

	if mgr.ValidatePQCAuth(sessionID) {
		t.Fatal("expected expired session to be invalid")
	}

	// Session should have been cleaned up
	mgr.mu.RLock()
	_, exists := mgr.sessions[sessionID]
	mgr.mu.RUnlock()
	if exists {
		t.Fatal("expected expired session to be deleted")
	}
}

func TestPQCKeyRotation(t *testing.T) {
	mgr, err := NewPQCManager()
	if err != nil {
		t.Fatalf("failed to create PQC manager: %v", err)
	}
	defer mgr.StopJanitor()

	originalKeyID := mgr.GetKeyInfo().KeyID
	originalPubKey := mgr.GetPublicKeyB64()

	err = mgr.RotateKey("test-actor")
	if err != nil {
		t.Fatalf("rotate key failed: %v", err)
	}

	newKeyID := mgr.GetKeyInfo().KeyID
	newPubKey := mgr.GetPublicKeyB64()

	if newKeyID == originalKeyID {
		t.Fatal("expected new key ID after rotation")
	}
	if newPubKey == originalPubKey {
		t.Fatal("expected new public key after rotation")
	}

	keyInfo := mgr.GetKeyInfo()
	if keyInfo.RotationActor != "test-actor" {
		t.Fatalf("expected rotation actor 'test-actor', got %s", keyInfo.RotationActor)
	}
}
