package security

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"
)

func TestPasskeyChallengeCreation(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-001"

	ch, err := pm.CreateChallenge(userID)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	if ch.ChallengeID == "" {
		t.Fatal("expected non-empty challenge ID")
	}

	if len(ch.Challenge) != challengeLength {
		t.Fatalf("expected %d byte challenge, got %d", challengeLength, len(ch.Challenge))
	}

	if ch.UserID != userID {
		t.Fatalf("expected user ID %q, got %q", userID, ch.UserID)
	}

	if ch.ExpiresAt.Before(ch.CreatedAt) {
		t.Fatal("expiry should be after creation")
	}
}

func TestPasskeyRegistrationAndVerification(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-002"

	privKey, pubBytes, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	keyID := "passkey-001"
	err = pm.Register(userID, keyID, "iPhone 15 Pro", pubBytes)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if !pm.HasRegistration(userID) {
		t.Fatal("expected HasRegistration to return true")
	}

	ch, err := pm.CreateChallenge(userID)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	digest := sha256.Sum256(ch.Challenge)
	signature, err := ecdsa.SignASN1(rand.Reader, privKey, digest[:])
	if err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	valid, err := pm.VerifyChallenge(userID, ch.ChallengeID, signature, keyID)
	if err != nil {
		t.Fatalf("VerifyChallenge failed: %v", err)
	}

	if !valid {
		t.Fatal("expected valid signature")
	}

	regs := pm.ListRegistrations(userID)
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}

	if regs[0].SignCount != 1 {
		t.Fatalf("expected sign count 1, got %d", regs[0].SignCount)
	}
}

func TestExpiredChallengeRejection(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-003"

	privKey, pubBytes, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	err = pm.Register(userID, "key-001", "Pixel 8", pubBytes)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ch, err := pm.CreateChallenge(userID)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	// Manually expire the challenge
	pm.mu.Lock()
	ch.ExpiresAt = time.Now().Add(-1 * time.Second)
	pm.mu.Unlock()

	digest := sha256.Sum256(ch.Challenge)
	signature, err := ecdsa.SignASN1(rand.Reader, privKey, digest[:])
	if err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	valid, err := pm.VerifyChallenge(userID, ch.ChallengeID, signature, "key-001")
	if err != ErrChallengeExpired {
		t.Fatalf("expected ErrChallengeExpired, got: %v", err)
	}

	if valid {
		t.Fatal("expected invalid result for expired challenge")
	}
}

func TestWrongSignatureRejection(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-004"

	_, pubBytes, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	err = pm.Register(userID, "key-correct", "MacBook Pro", pubBytes)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Generate a different key pair for signing (simulating attacker)
	wrongPrivKey, _, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("wrong key generation failed: %v", err)
	}

	ch, err := pm.CreateChallenge(userID)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	digest := sha256.Sum256(ch.Challenge)
	signature, err := ecdsa.SignASN1(rand.Reader, wrongPrivKey, digest[:])
	if err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	valid, err := pm.VerifyChallenge(userID, ch.ChallengeID, signature, "key-correct")
	if err != nil {
		t.Fatalf("VerifyChallenge returned unexpected error: %v", err)
	}

	if valid {
		t.Fatal("expected invalid signature to be rejected")
	}
}

func TestReplayAttackRejection(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-005"

	privKey, pubBytes, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	err = pm.Register(userID, "key-001", "iPad Pro", pubBytes)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ch, err := pm.CreateChallenge(userID)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	digest := sha256.Sum256(ch.Challenge)
	signature, err := ecdsa.SignASN1(rand.Reader, privKey, digest[:])
	if err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	// First verification succeeds
	valid, err := pm.VerifyChallenge(userID, ch.ChallengeID, signature, "key-001")
	if err != nil {
		t.Fatalf("VerifyChallenge failed: %v", err)
	}

	if !valid {
		t.Fatal("expected first verification to succeed")
	}

	// Replay should fail
	valid, err = pm.VerifyChallenge(userID, ch.ChallengeID, signature, "key-001")
	if err != ErrChallengeUsed {
		t.Fatalf("expected ErrChallengeUsed on replay, got: %v", err)
	}

	if valid {
		t.Fatal("expected replay to be rejected")
	}
}

func TestRemoveRegistration(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-006"

	_, pubBytes, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	err = pm.Register(userID, "key-001", "Samsung S24", pubBytes)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if !pm.HasRegistration(userID) {
		t.Fatal("expected registration to exist")
	}

	err = pm.RemoveRegistration(userID, "key-001")
	if err != nil {
		t.Fatalf("RemoveRegistration failed: %v", err)
	}

	if pm.HasRegistration(userID) {
		t.Fatal("expected no registrations after removal")
	}

	// Removing again should return error
	err = pm.RemoveRegistration(userID, "key-001")
	if err != ErrKeyIDNotFound {
		t.Fatalf("expected ErrKeyIDNotFound, got: %v", err)
	}
}

func TestDuplicateRegistrationRejection(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-007"

	_, pubBytes, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	err = pm.Register(userID, "key-dup", "Device 1", pubBytes)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Registering the same key ID again should fail
	err = pm.Register(userID, "key-dup", "Device 2", pubBytes)
	if err != ErrRegistrationExists {
		t.Fatalf("expected ErrRegistrationExists, got: %v", err)
	}
}

func TestWrongUserIDRejection(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-correct"

	_, pubBytes, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	err = pm.Register(userID, "key-001", "Device", pubBytes)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	wrongPrivKey, _, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	ch, err := pm.CreateChallenge(userID)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	digest := sha256.Sum256(ch.Challenge)
	signature, err := ecdsa.SignASN1(rand.Reader, wrongPrivKey, digest[:])
	if err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	// Try to verify with wrong user ID (but valid keyID)
	valid, err := pm.VerifyChallenge("user-wrong", ch.ChallengeID, signature, "key-001")
	if err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}

	if valid {
		t.Fatal("expected failure when user ID mismatch")
	}
}

func TestNonexistentKeyIDRejection(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-009"

	privKey, pubBytes, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	err = pm.Register(userID, "key-real", "Device", pubBytes)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ch, err := pm.CreateChallenge(userID)
	if err != nil {
		t.Fatalf("CreateChallenge failed: %v", err)
	}

	digest := sha256.Sum256(ch.Challenge)
	signature, err := ecdsa.SignASN1(rand.Reader, privKey, digest[:])
	if err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	valid, err := pm.VerifyChallenge(userID, ch.ChallengeID, signature, "key-nonexistent")
	if err != ErrKeyIDNotFound {
		t.Fatalf("expected ErrKeyIDNotFound, got: %v", err)
	}

	if valid {
		t.Fatal("expected failure with nonexistent key ID")
	}
}

func TestMultipleRegistrationsPerUser(t *testing.T) {
	pm := NewPasskeyManager()
	userID := "user-multi"

	_, pub1, _ := GeneratePasskeyKeyPair()
	_, pub2, _ := GeneratePasskeyKeyPair()
	_, pub3, _ := GeneratePasskeyKeyPair()

	pm.Register(userID, "key-1", "iPhone", pub1)
	pm.Register(userID, "key-2", "Android", pub2)
	pm.Register(userID, "key-3", "MacBook", pub3)

	regs := pm.ListRegistrations(userID)
	if len(regs) != 3 {
		t.Fatalf("expected 3 registrations, got %d", len(regs))
	}
}

func TestMarshalUnmarshalPublicKey(t *testing.T) {
	privKey, pubBytes, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}

	parsed, err := parseECDSAPublicKey(pubBytes)
	if err != nil {
		t.Fatalf("parseECDSAPublicKey failed: %v", err)
	}

	if parsed.X.Cmp(privKey.PublicKey.X) != 0 {
		t.Fatal("X coordinate mismatch after marshal/unmarshal")
	}

	if parsed.Y.Cmp(privKey.PublicKey.Y) != 0 {
		t.Fatal("Y coordinate mismatch after marshal/unmarshal")
	}
}
