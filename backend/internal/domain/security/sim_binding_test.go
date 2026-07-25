package security

import (
	"testing"
	"time"
)

func TestSIMBindingRoundtrip(t *testing.T) {
	m := NewSIMBindingManager()
	const (
		userID      = "usr_1"
		mobile      = "+919983692606"
		deviceID    = "dev_abc123"
		wrongDevice = "dev_WRONG"
		simICCID    = "8991001234567890"
	)

	ch, err := m.CreateBindingChallenge(userID, mobile, deviceID)
	if err != nil {
		t.Fatalf("CreateBindingChallenge: %v", err)
	}
	if ch.ChallengeID == "" || ch.VNNCode == "" {
		t.Fatal("challenge missing id or VNN code")
	}

	rec, err := m.VerifyBindingChallenge(ch.ChallengeID, ch.VNNCode, simICCID)
	if err != nil {
		t.Fatalf("VerifyBindingChallenge (happy): %v", err)
	}
	// Core bug fix: SIM must bind to the actual device, not the user-id string.
	if rec.DeviceID != deviceID {
		t.Fatalf("DeviceID = %q, want %q (was binding to UserID=%q)", rec.DeviceID, deviceID, userID)
	}
	if rec.DeviceID == userID {
		t.Fatalf("DeviceID equals UserID %q — bug not fixed", userID)
	}
	if !rec.Verified || rec.UBT == "" {
		t.Fatal("binding not verified or UBT empty")
	}

	ok, err := m.VerifyUBT(userID, rec.UBT)
	if err != nil || !ok {
		t.Fatalf("VerifyUBT: ok=%v err=%v", ok, err)
	}
	// Wrong UBT must fail.
	if ok, _ := m.VerifyUBT(userID, "ubt_bogus"); ok {
		t.Fatal("VerifyUBT accepted a wrong UBT")
	}
}

func TestSIMBindingNegative(t *testing.T) {
	m := NewSIMBindingManager()
	const (
		userID   = "usr_2"
		mobile   = "+916303891930"
		deviceID = "dev_xyz"
	)

	ch, err := m.CreateBindingChallenge(userID, mobile, deviceID)
	if err != nil {
		t.Fatalf("CreateBindingChallenge: %v", err)
	}

	// Wrong VNN code rejected.
	if _, err := m.VerifyBindingChallenge(ch.ChallengeID, "VNN-wrong", "8991"); err == nil {
		t.Fatal("expected VNN mismatch error, got nil")
	}

	// Re-used challenge rejected.
	if _, err := m.VerifyBindingChallenge(ch.ChallengeID, ch.VNNCode, "8991"); err != nil {
		t.Fatalf("first verify failed: %v", err)
	}
	if _, err := m.VerifyBindingChallenge(ch.ChallengeID, ch.VNNCode, "8991"); err == nil {
		t.Fatal("expected already-used challenge error, got nil")
	}

	// Unknown challenge rejected.
	if _, err := m.VerifyBindingChallenge("nope", ch.VNNCode, "8991"); err == nil {
		t.Fatal("expected unknown challenge error, got nil")
	}
}

func TestSIMBindingExpired(t *testing.T) {
	m := NewSIMBindingManager()
	// Exercise the expiry branch without waiting out the 10-minute TTL: confirm a
	// fresh challenge is not already expired, then insert a pre-expired challenge
	// directly and assert VerifyBindingChallenge rejects it.
	ch, err := m.CreateBindingChallenge("usr_3", "+910000000000", "dev_t")
	if err != nil {
		t.Fatalf("CreateBindingChallenge: %v", err)
	}
	if !ch.ExpiresAt.After(time.Now().UTC()) {
		t.Fatal("fresh challenge should not be expired")
	}
	// Insert an expired challenge directly to exercise the expiry branch.
	expired := &SIMBindingChallenge{
		ChallengeID:  "exp_1",
		UserID:       "usr_3",
		MobileNumber: "+910000000000",
		VNNCode:      "VNN-dead",
		CreatedAt:    time.Now().UTC().Add(-time.Hour),
		ExpiresAt:    time.Now().UTC().Add(-time.Minute),
	}
	m.mu.Lock()
	m.challenges[expired.ChallengeID] = expired
	m.mu.Unlock()

	if _, err := m.VerifyBindingChallenge("exp_1", "VNN-dead", "8991"); err == nil {
		t.Fatal("expected expired challenge error, got nil")
	}
}
