package security

import (
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// JWT auth per spec §3.6: short-lived (15-min) HS256 tokens replace the previous
// opaque session tokens. The TEE-bound signing key is a client concern; the
// backend signs and verifies with a server secret (FINIX_JWT_SECRET).
//
// Implemented with the stdlib (no external JWT dependency): a compact
// header.payload.signature triple, HMAC-SHA256 over the signing input.

const (
	// TokenTTL is the 15-minute inactivity/session expiry from §3.6.
	TokenTTL = 15 * time.Minute
	// RefreshGrace is how long after expiry a token may still be refreshed
	// (rather than forcing a full re-login).
	RefreshGrace = 5 * time.Minute
	// maxRefreshableAge caps the total age (since issue) of a token that may be
	// refreshed. Beyond this a full re-login is required, so a stolen expired token
	// cannot be refreshed indefinitely (M-7).
	maxRefreshableAge = 24 * time.Hour
)

// Typed errors let the middleware return a structured code (e.g. token_expired)
// so the Flutter interceptor can silently re-auth vs. force logout.
var (
	ErrTokenExpired   = errors.New("token_expired")
	ErrTokenInvalid   = errors.New("token_invalid")
	ErrDeviceMismatch = errors.New("device_mismatch")
	ErrTokenRevoked   = errors.New("token_revoked")
	ErrJWTSecretUnset = errors.New("FINIX_JWT_SECRET is not set (no insecure default is permitted)")
	ErrRefreshTooLate = errors.New("refresh window elapsed; re-login required")
)

// Claims is the JWT payload.
type Claims struct {
	Sub      string `json:"sub"`       // userID
	Role     string `json:"role"`      // e.g. customer/admin
	DeviceFP string `json:"device_fp"` // bound device fingerprint
	IAT      int64  `json:"iat"`
	EXP      int64  `json:"exp"`
	JTI      string `json:"jti"`
}

// JWTManager issues and verifies HS256 tokens and tracks a revocation set.
//
// Always construct via NewJWTManager / NewJWTManagerWithSecret: the zero value
// has a nil revocation map and would panic on the first revoke.
type JWTManager struct {
	secret  []byte
	mu      sync.RWMutex
	revoked map[string]int64 // jti -> exp (unix); pruned lazily
	// persist is an optional hook to persist a revocation to Postgres.
	persist func(jti string, exp time.Time)
	// lookup is an optional hook to check revocation in Postgres (survives
	// restart). Consulted only on an in-memory miss so the hot path stays fast.
	lookup func(jti string) (bool, error)
	// failClosed selects the policy when the revocation lookup itself errors:
	// true → deny the token (fail-closed), false → treat as not-revoked (fail-open).
	failClosed bool
}

// NewJWTManager reads FINIX_JWT_SECRET and returns an error if it is unset, so
// startup can refuse rather than fall back to an insecure default.
func NewJWTManager() (*JWTManager, error) {
	secret := strings.TrimSpace(os.Getenv("FINIX_JWT_SECRET"))
	if secret == "" {
		return nil, ErrJWTSecretUnset
	}
	if len(secret) < 16 {
		return nil, fmt.Errorf("FINIX_JWT_SECRET must be at least 16 chars")
	}
	return &JWTManager{
		secret:  []byte(secret),
		revoked: make(map[string]int64),
	}, nil
}

// NewJWTManagerWithSecret builds a manager from an explicit secret (tests).
func NewJWTManagerWithSecret(secret string) *JWTManager {
	return &JWTManager{secret: []byte(secret), revoked: make(map[string]int64)}
}

// SetRevocationPersister wires an optional Postgres-backed revocation writer.
func (m *JWTManager) SetRevocationPersister(f func(jti string, exp time.Time)) {
	m.mu.Lock()
	m.persist = f
	m.mu.Unlock()
}

// SetRevocationLookup wires an optional Postgres-backed revocation reader so a
// revocation survives a process restart (the in-memory set is lost on restart).
func (m *JWTManager) SetRevocationLookup(f func(jti string) (bool, error)) {
	m.mu.Lock()
	m.lookup = f
	m.mu.Unlock()
}

// SetRevocationFailClosed selects the policy when the revocation lookup errors:
// true denies the token, false accepts it (bounded by the 15-min TTL). Set once
// at startup, before serving.
func (m *JWTManager) SetRevocationFailClosed(v bool) {
	m.mu.Lock()
	m.failClosed = v
	m.mu.Unlock()
}

// IssueToken mints a 15-minute HS256 token bound to userID/role/deviceFP.
func (m *JWTManager) IssueToken(userID, role, deviceFP string) (string, error) {
	if userID == "" {
		return "", errors.New("userID is required")
	}
	jti, err := randomHexJWT(16)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	claims := Claims{
		Sub:      userID,
		Role:     firstNonEmptyJWT(role, "customer"),
		DeviceFP: deviceFP,
		IAT:      now.Unix(),
		EXP:      now.Add(TokenTTL).Unix(),
		JTI:      "jti_" + jti,
	}
	return m.sign(claims)
}

// VerifyToken validates the signature, expiry and device binding. It returns
// typed errors so callers can distinguish expiry / device mismatch / invalid.
func (m *JWTManager) VerifyToken(token, expectedDeviceFP string) (*Claims, error) {
	claims, err := m.parse(token)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if now >= claims.EXP {
		return nil, ErrTokenExpired
	}
	if m.revocationDenies(claims.JTI) {
		return nil, ErrTokenRevoked
	}
	// Device binding: if the token is bound to a device, the request must match.
	if claims.DeviceFP != "" {
		if strings.TrimSpace(expectedDeviceFP) == "" || expectedDeviceFP != claims.DeviceFP {
			return nil, ErrDeviceMismatch
		}
	}
	return claims, nil
}

// RefreshToken issues a fresh token if the presented one is valid or expired but
// still inside the refresh grace window; otherwise re-login is required.
func (m *JWTManager) RefreshToken(token, deviceFP string) (string, error) {
	claims, err := m.parse(token)
	if err != nil {
		return "", err
	}
	if m.revocationDenies(claims.JTI) {
		return "", ErrTokenRevoked
	}
	if claims.DeviceFP != "" && deviceFP != claims.DeviceFP {
		return "", ErrDeviceMismatch
	}
	now := time.Now().Unix()
	if now >= claims.EXP+int64(RefreshGrace.Seconds()) {
		return "", ErrRefreshTooLate
	}
	// M-7 fix: cap how old a token may be to refresh, so a stolen expired token
	// pulled from logs/cache cannot be refreshed indefinitely. A token issued more
	// than maxRefreshableAge ago must go through a full re-login.
	if claims.IAT > 0 && now-claims.IAT > int64(maxRefreshableAge.Seconds()) {
		return "", ErrRefreshTooLate
	}
	// M-2 fix: atomically claim the old jti for rotation. Only the first of any
	// concurrent refreshes of the same token wins; the rest see it already
	// revoked and must re-login. This closes the check-then-revoke race that let
	// one token mint several.
	if !m.claimForRotation(claims.JTI) {
		return "", ErrTokenRevoked
	}
	return m.IssueToken(claims.Sub, claims.Role, claims.DeviceFP)
}

// RevokeToken adds a jti to the revocation set (and persists it if configured).
func (m *JWTManager) RevokeToken(jti string) {
	if jti == "" {
		return
	}
	exp := time.Now().Add(TokenTTL + RefreshGrace)
	m.mu.Lock()
	m.revoked[jti] = exp.Unix()
	persist := m.persist
	m.pruneLocked()
	m.mu.Unlock()
	if persist != nil {
		persist(jti, exp)
	}
}

// RevokeTokenString revokes by the full token (convenience for logout).
func (m *JWTManager) RevokeTokenString(token string) {
	if claims, err := m.parse(token); err == nil {
		m.RevokeToken(claims.JTI)
	}
}

// claimForRotation atomically claims a jti for single-use rotation: it returns
// true only for the FIRST caller, revoking the jti in the same critical section.
// M-2 fix: RefreshToken previously checked revocation and revoked in separate
// locked regions, so two concurrent refreshes of the same token could both pass
// the check and each mint a fresh token (token doubling). Folding the
// check-and-revoke into one lock makes rotation exactly-once; the loser gets
// false and must re-login.
func (m *JWTManager) claimForRotation(jti string) bool {
	if jti == "" {
		return false
	}
	exp := time.Now().Add(TokenTTL + RefreshGrace)
	m.mu.Lock()
	if _, already := m.revoked[jti]; already {
		m.mu.Unlock()
		return false
	}
	m.revoked[jti] = exp.Unix()
	persist := m.persist
	m.pruneLocked()
	m.mu.Unlock()
	if persist != nil {
		persist(jti, exp)
	}
	return true
}

// ── internals ───────────────────────────────────────────────

// isRevoked reports whether jti is revoked. On an in-memory miss it consults the
// optional Postgres lookup (so a revocation issued before a restart still
// denies); a lookup error is surfaced, not swallowed, so the caller can apply the
// fail-open/closed policy.
func (m *JWTManager) isRevoked(jti string) (bool, error) {
	m.mu.RLock()
	_, ok := m.revoked[jti]
	lookup := m.lookup
	m.mu.RUnlock()
	if ok {
		return true, nil
	}
	if lookup != nil {
		return lookup(jti)
	}
	return false, nil
}

// revocationDenies applies the configured policy: an in-memory or Postgres hit
// always denies; a lookup error denies only when failClosed is set. The error is
// logged either way so a DB blip on this security-relevant path is visible.
func (m *JWTManager) revocationDenies(jti string) bool {
	revoked, err := m.isRevoked(jti)
	if err != nil {
		m.mu.RLock()
		fc := m.failClosed
		m.mu.RUnlock()
		slog.Warn("jwt revocation lookup failed", "error", err, "failClosed", fc)
		return fc
	}
	return revoked
}

func (m *JWTManager) pruneLocked() {
	now := time.Now().Unix()
	for jti, exp := range m.revoked {
		if exp < now {
			delete(m.revoked, jti)
		}
	}
}

func (m *JWTManager) sign(claims Claims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := header + "." + payload
	sig := m.hmac(signingInput)
	return signingInput + "." + sig, nil
}

func (m *JWTManager) parse(token string) (*Claims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return nil, ErrTokenInvalid
	}
	signingInput := parts[0] + "." + parts[1]
	expectedSig := m.hmac(signingInput)
	// Constant-time comparison to avoid timing oracles.
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expectedSig)) != 1 {
		return nil, ErrTokenInvalid
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrTokenInvalid
	}
	if claims.Sub == "" || claims.EXP == 0 {
		return nil, ErrTokenInvalid
	}
	return &claims, nil
}

func (m *JWTManager) hmac(input string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func firstNonEmptyJWT(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func randomHexJWT(n int) (string, error) {
	b := make([]byte, n)
	if _, err := cryptorand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return hex.EncodeToString(b), nil
}
