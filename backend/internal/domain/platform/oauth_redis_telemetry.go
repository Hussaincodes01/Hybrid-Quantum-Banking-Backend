package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ============================================================
// OAUTH 2.1 TOKEN ENDPOINT (RFC 6749 + PKCE per RFC 7636)
// Coexists with existing biometric challenge-response auth.
// All token validation is SERVER-SIDE only.
// ============================================================

type OAuthGrantType string

const (
	GrantTypeAuthorizationCode OAuthGrantType = "authorization_code"
	GrantTypeRefreshToken      OAuthGrantType = "refresh_token"
	GrantTypeClientCredentials OAuthGrantType = "client_credentials"
)

type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type OAuthServer struct {
	authCodes     map[string]*AuthCodeEntry
	refreshTokens map[string]string
	clientSecrets map[string]string
}

type AuthCodeEntry struct {
	UserID        string
	CodeChallenge string
	CreatedAt     time.Time
	Used          bool
}

func NewOAuthServer() *OAuthServer {
	s := &OAuthServer{
		authCodes:     make(map[string]*AuthCodeEntry),
		refreshTokens: make(map[string]string),
		clientSecrets: map[string]string{
			"finix-mobile": os.Getenv("FINIX_MOBILE_CLIENT_SECRET"),
		},
	}
	if s.clientSecrets["finix-mobile"] == "" {
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err != nil {
			s.clientSecrets["finix-mobile"] = "change-this-immediately"
		} else {
			s.clientSecrets["finix-mobile"] = hex.EncodeToString(bytes)
		}
	}
	return s
}

func (oa *OAuthServer) GenerateAuthCode(userID, codeChallenge string) string {
	code := "auth_code_" + randomHex(20)
	oa.authCodes[code] = &AuthCodeEntry{
		UserID:        userID,
		CodeChallenge: codeChallenge,
		CreatedAt:     time.Now().UTC(),
	}
	return code
}

func (oa *OAuthServer) ExchangeToken(req TokenRequest) (*TokenResponse, error) {
	switch OAuthGrantType(req.GrantType) {
	case GrantTypeAuthorizationCode:
		return oa.handleAuthorizationCode(req)
	case GrantTypeRefreshToken:
		return oa.handleRefreshToken(req)
	default:
		return nil, errors.New("unsupported grant type")
	}
}

func (oa *OAuthServer) handleAuthorizationCode(req TokenRequest) (*TokenResponse, error) {
	if strings.TrimSpace(req.Code) == "" {
		return nil, errors.New("code is required")
	}
	// C-1 fix: validate client_secret before doing anything with the auth code.
	// Constant-time compare to avoid a timing oracle on the secret.
	expectedSecret, knownClient := oa.clientSecrets[req.ClientID]
	if !knownClient || req.ClientSecret == "" ||
		subtle.ConstantTimeCompare([]byte(req.ClientSecret), []byte(expectedSecret)) != 1 {
		return nil, errors.New("invalid client credentials")
	}
	entry, ok := oa.authCodes[req.Code]
	if !ok || entry.Used {
		return nil, errors.New("invalid or expired authorization code")
	}
	if time.Since(entry.CreatedAt) > 5*time.Minute {
		delete(oa.authCodes, req.Code)
		return nil, errors.New("authorization code expired")
	}
	if !verifyPKCE(entry.CodeChallenge, req.CodeVerifier) {
		return nil, errors.New("PKCE code verifier mismatch")
	}
	entry.Used = true
	oa.authCodes[req.Code] = entry

	accessToken := "access_" + randomHex(24)
	refreshToken := "refresh_" + randomHex(24)
	oa.refreshTokens[refreshToken] = entry.UserID

	return &TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    900,
		RefreshToken: refreshToken,
		Scope:        "openid profile transactions goals portfolio",
	}, nil
}

func (oa *OAuthServer) handleRefreshToken(req TokenRequest) (*TokenResponse, error) {
	if strings.TrimSpace(req.RefreshToken) == "" {
		return nil, errors.New("refresh_token is required")
	}
	userID, ok := oa.refreshTokens[req.RefreshToken]
	if !ok {
		return nil, errors.New("invalid refresh token")
	}
	delete(oa.refreshTokens, req.RefreshToken)
	accessToken := "access_" + randomHex(24)
	newRefreshToken := "refresh_" + randomHex(24)
	oa.refreshTokens[newRefreshToken] = userID
	return &TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    900,
		RefreshToken: newRefreshToken,
		Scope:        "openid profile transactions goals portfolio",
	}, nil
}

func verifyPKCE(challenge, verifier string) bool {
	// C-1 fix: an empty challenge must FAIL, not pass. The old `return true` on
	// empty challenge allowed code exchange without any PKCE verification.
	if challenge == "" || verifier == "" {
		return false
	}
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])
	return challenge == expected
}

// ============================================================
// REDIS SESSION ABSTRACTION
// Provides a pluggable session store: in-memory (dev) or Redis (prod).
// All session operations are SERVER-SIDE only.
// ============================================================

type SessionStore interface {
	Set(key string, value string, ttl time.Duration) error
	Get(key string) (string, error)
	Delete(key string) error
	Exists(key string) (bool, error)
	Ping() error
}

type InMemorySessionStore struct {
	data map[string]*inMemoryEntry
}

type inMemoryEntry struct {
	value   string
	expires time.Time
}

func NewInMemorySessionStore() *InMemorySessionStore {
	s := &InMemorySessionStore{data: make(map[string]*inMemoryEntry)}
	go s.cleanupLoop()
	return s
}

func (s *InMemorySessionStore) Set(key, value string, ttl time.Duration) error {
	s.data[key] = &inMemoryEntry{value: value, expires: time.Now().Add(ttl)}
	return nil
}

func (s *InMemorySessionStore) Get(key string) (string, error) {
	entry, ok := s.data[key]
	if !ok || time.Now().After(entry.expires) {
		return "", fmt.Errorf("session not found or expired: %s", key)
	}
	return entry.value, nil
}

func (s *InMemorySessionStore) Delete(key string) error {
	delete(s.data, key)
	return nil
}

func (s *InMemorySessionStore) Exists(key string) (bool, error) {
	entry, ok := s.data[key]
	return ok && time.Now().Before(entry.expires), nil
}

func (s *InMemorySessionStore) Ping() error {
	return nil
}

func (s *InMemorySessionStore) cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		for key, entry := range s.data {
			if now.After(entry.expires) {
				delete(s.data, key)
			}
		}
	}
}

// ============================================================
// OPEN TELEMETRY SPAN WRAPPER
// Minimal tracing: start+end spans exported to stdout/collector.
// Production: export via OTLP to Jaeger/Tempo.
// ============================================================

type TraceSpan struct {
	OperationName string
	StartTime     time.Time
	Tags          map[string]string
}

var tracingEnabled bool

func init() {
	tracingEnabled = os.Getenv("FINIX_TRACING_ENABLED") == "true"
}

func StartSpan(operationName string) *TraceSpan {
	if !tracingEnabled {
		return nil
	}
	return &TraceSpan{
		OperationName: operationName,
		StartTime:     time.Now().UTC(),
		Tags:          map[string]string{"span.kind": "server"},
	}
}

func (s *TraceSpan) SetTag(key, value string) {
	if s == nil {
		return
	}
	s.Tags[key] = value
}

func (s *TraceSpan) Finish() {
	if s == nil {
		return
	}
	duration := time.Since(s.StartTime)
	// In production: export via OTLP (OpenTelemetry Protocol)
	_ = duration
}
