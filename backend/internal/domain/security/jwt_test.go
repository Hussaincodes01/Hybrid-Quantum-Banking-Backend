package security

import (
	"os"
	"strings"
	"testing"
	"time"
)

const testSecret = "test-secret-at-least-16-chars-long"

func TestJWTIssueAndVerify(t *testing.T) {
	m := NewJWTManagerWithSecret(testSecret)
	tok, err := m.IssueToken("usr_1", "customer", "device-abc")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := m.VerifyToken(tok, "device-abc")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Sub != "usr_1" || claims.Role != "customer" || claims.DeviceFP != "device-abc" {
		t.Errorf("claims mismatch: %+v", claims)
	}
	if claims.JTI == "" {
		t.Error("expected a jti")
	}
}

func TestJWTRejectsTampered(t *testing.T) {
	m := NewJWTManagerWithSecret(testSecret)
	tok, _ := m.IssueToken("usr_1", "customer", "device-abc")

	// Flip a character in the payload segment.
	parts := strings.Split(tok, ".")
	parts[1] = parts[1][:len(parts[1])-1] + "X"
	if _, err := m.VerifyToken(strings.Join(parts, "."), "device-abc"); err != ErrTokenInvalid {
		t.Errorf("tampered token: got %v, want ErrTokenInvalid", err)
	}

	// A token signed with a different secret must not verify.
	other := NewJWTManagerWithSecret("another-secret-16-chars-xx")
	otherTok, _ := other.IssueToken("usr_1", "customer", "device-abc")
	if _, err := m.VerifyToken(otherTok, "device-abc"); err != ErrTokenInvalid {
		t.Errorf("foreign-signed token: got %v, want ErrTokenInvalid", err)
	}
}

func TestJWTDeviceMismatch(t *testing.T) {
	m := NewJWTManagerWithSecret(testSecret)
	tok, _ := m.IssueToken("usr_1", "customer", "device-abc")
	if _, err := m.VerifyToken(tok, "device-WRONG"); err != ErrDeviceMismatch {
		t.Errorf("device mismatch: got %v, want ErrDeviceMismatch", err)
	}
	if _, err := m.VerifyToken(tok, ""); err != ErrDeviceMismatch {
		t.Errorf("empty device: got %v, want ErrDeviceMismatch", err)
	}
}

func TestJWTExpiry(t *testing.T) {
	m := NewJWTManagerWithSecret(testSecret)
	// Hand-craft a token that expired 2 minutes ago — still inside the 5-minute
	// refresh grace window.
	expired := Claims{
		Sub: "usr_1", Role: "customer", DeviceFP: "device-abc",
		IAT: time.Now().Add(-17 * time.Minute).Unix(),
		EXP: time.Now().Add(-2 * time.Minute).Unix(),
		JTI: "jti_expired",
	}
	tok, err := m.sign(expired)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := m.VerifyToken(tok, "device-abc"); err != ErrTokenExpired {
		t.Errorf("expired token: got %v, want ErrTokenExpired", err)
	}

	// Still refreshable within the grace window.
	if _, err := m.RefreshToken(tok, "device-abc"); err != nil {
		t.Errorf("refresh within grace: %v", err)
	}
}

func TestJWTRefreshTooLate(t *testing.T) {
	m := NewJWTManagerWithSecret(testSecret)
	tooOld := Claims{
		Sub: "usr_1", DeviceFP: "d",
		EXP: time.Now().Add(-(TokenTTL + RefreshGrace + time.Minute)).Unix(),
		JTI: "jti_old",
	}
	tok, _ := m.sign(tooOld)
	if _, err := m.RefreshToken(tok, "d"); err != ErrRefreshTooLate {
		t.Errorf("late refresh: got %v, want ErrRefreshTooLate", err)
	}
}

func TestJWTRevocation(t *testing.T) {
	m := NewJWTManagerWithSecret(testSecret)
	tok, _ := m.IssueToken("usr_1", "customer", "device-abc")
	claims, _ := m.VerifyToken(tok, "device-abc")
	m.RevokeToken(claims.JTI)
	if _, err := m.VerifyToken(tok, "device-abc"); err != ErrTokenRevoked {
		t.Errorf("revoked token: got %v, want ErrTokenRevoked", err)
	}
}

func TestNewJWTManagerRequiresSecret(t *testing.T) {
	old := os.Getenv("FINIX_JWT_SECRET")
	_ = os.Unsetenv("FINIX_JWT_SECRET")
	defer func() {
		if old != "" {
			_ = os.Setenv("FINIX_JWT_SECRET", old)
		}
	}()
	if _, err := NewJWTManager(); err != ErrJWTSecretUnset {
		t.Errorf("unset secret: got %v, want ErrJWTSecretUnset", err)
	}

	_ = os.Setenv("FINIX_JWT_SECRET", "short")
	if _, err := NewJWTManager(); err == nil {
		t.Error("expected an error for a too-short secret")
	}
}
