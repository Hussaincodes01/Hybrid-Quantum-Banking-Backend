// Package sim provides SIM-binding / VNN verification providers for FINIX.
//
// MockSIM is the DEFAULT provider. It generates a VNN-style challenge with a
// short TTL and simulates the telecom callback on verify, so the seam behaves
// like a real integration without a telecom API. A real provider (telecom.go)
// can be swapped in by env with no call-site changes.
package sim

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"FINIX/backend/internal/domain/security"
)

// ErrNotImplemented marks a real provider that is not yet wired.
var ErrNotImplemented = errors.New("sim provider not implemented: real integration not wired")

const challengeTTL = 3 * time.Minute

type challenge struct {
	mobile  string
	code    string
	expires time.Time
}

// MockSIM is an offline SIMVerifier.
type MockSIM struct {
	mu         sync.Mutex
	challenges map[string]challenge // challengeID -> challenge
	swapped    map[string]bool      // userID -> forced swap (test hook)
}

var _ security.SIMVerifier = (*MockSIM)(nil)

// NewMockSIM returns the default mock as the interface type.
func NewMockSIM() security.SIMVerifier {
	return &MockSIM{
		challenges: make(map[string]challenge),
		swapped:    make(map[string]bool),
	}
}

// CreateChallenge issues a VNN-style challenge for the mobile and returns its id.
func (m *MockSIM) CreateChallenge(ctx context.Context, mobile string) (string, error) {
	mobile = strings.TrimSpace(mobile)
	if mobile == "" {
		return "", fmt.Errorf("mobile is required")
	}
	id := "vnnch_" + randomHex(8)
	m.mu.Lock()
	m.challenges[id] = challenge{
		mobile:  mobile,
		code:    fmt.Sprintf("VNN%06d", randomInt(1000000)),
		expires: time.Now().Add(challengeTTL),
	}
	m.mu.Unlock()
	return id, nil
}

// VerifyChallenge simulates the telecom callback: a live, non-expired challenge
// with a supplied code is confirmed. (A real provider would match the exact
// code the SIM received; the mock treats the telecom as having validated it.)
func (m *MockSIM) VerifyChallenge(ctx context.Context, mobile, challengeID, code string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.challenges[challengeID]
	if !ok {
		return false, nil
	}
	if time.Now().After(ch.expires) {
		delete(m.challenges, challengeID)
		return false, nil
	}
	if strings.TrimSpace(mobile) != "" && ch.mobile != strings.TrimSpace(mobile) {
		return false, nil
	}
	if strings.TrimSpace(code) == "" {
		return false, nil
	}
	delete(m.challenges, challengeID) // one-time use
	return true, nil
}

// DetectSwap reports a SIM swap. Defaults to false; SetSwap flips it for tests.
func (m *MockSIM) DetectSwap(ctx context.Context, userID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.swapped[userID], nil
}

// SetSwap forces DetectSwap to report a swap for a user (test/demo hook).
func (m *MockSIM) SetSwap(userID string, swapped bool) {
	m.mu.Lock()
	m.swapped[userID] = swapped
	m.mu.Unlock()
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(b)
}

func randomInt(max int) int {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return 0
	}
	v := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if v < 0 {
		v = -v
	}
	return v % max
}
