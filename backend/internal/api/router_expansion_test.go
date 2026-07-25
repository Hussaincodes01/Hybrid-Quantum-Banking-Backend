package api

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"FINIX/backend/internal/domain/security"
)

func TestExpandedRoutesAuthAndInternal(t *testing.T) {
	// Scoped tokens are randomised when unset; pin them for the test.
	t.Setenv("FINIX_INTERNAL_TOKEN", "FINIX-internal")
	t.Setenv("FINIX_ADMIN_TOKEN", "FINIX-admin")

	// H-5: admin routes now require BOTH the service token AND a JWT principal
	// holding a privileged role. Inject a JWT manager so the test can mint an
	// admin-role token; the opaque biometric-login tokens still verify via the
	// legacy fallback path, so the customer flows below are unaffected.
	jwtMgr := security.NewJWTManagerWithSecret("finix-router-expansion-test-secret")
	ts := httptest.NewServer(NewRouter(nil, WithJWTManager(jwtMgr)))
	defer ts.Close()

	// Register with device fingerprint
	keyPair, pubKeyB64, keyID := generateTestKeyPair(t)
	userID := mustRegisterAndLogin(t, ts.URL, "device-test-fp-1", pubKeyB64, keyID)
	token := mustLoginToken(t, ts.URL, userID, "device-test-fp-1", keyPair, keyID)
	accountID := mustFirstAccount(t, ts.URL, token, "device-test-fp-1")

	_ = accountID

	// Public SIM bind
	status, simBindPayload := callJSON(t, http.MethodPost, ts.URL+"/v1/auth/sim-bind", map[string]any{
		"mobile":    "9000000099",
		"device_id": "device-x",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("sim-bind status=%d payload=%v", status, simBindPayload)
	}

	// Bearer endpoint
	status, payload := callJSON(t, http.MethodGet, ts.URL+"/v1/dashboard", nil, map[string]string{
		"Authorization":        "Bearer " + token,
		"X-Device-Fingerprint": "device-test-fp-1",
	})
	if status != http.StatusOK {
		t.Fatalf("dashboard status=%d payload=%v", status, payload)
	}

	// Internal endpoint
	status, payload = callJSON(t, http.MethodPost, ts.URL+"/v1/internal/blockchain/consent-record", map[string]any{
		"user_id":          userID,
		"consent_event_id": "cons-1",
	}, map[string]string{"X-Internal-Token": "FINIX-internal"})
	if status != http.StatusOK {
		t.Fatalf("blockchain consent status=%d payload=%v", status, payload)
	}

	// Admin endpoint (H-5): the service token ALONE is no longer sufficient — a
	// privileged JWT principal is also required. Token-only must be rejected.
	status, payload = callJSON(t, http.MethodGet, ts.URL+"/v1/admin/aiml/bias-audit", nil, map[string]string{"X-Admin-Token": "FINIX-admin"})
	if status == http.StatusOK {
		t.Fatalf("bias-audit accepted with admin token but no privileged JWT (H-5 regression): payload=%v", payload)
	}

	// With BOTH the admin token AND an admin-role JWT it must succeed.
	adminJWT, err := jwtMgr.IssueToken("admin-user-1", "admin", "")
	if err != nil {
		t.Fatalf("mint admin jwt failed: %v", err)
	}
	status, payload = callJSON(t, http.MethodGet, ts.URL+"/v1/admin/aiml/bias-audit", nil, map[string]string{
		"X-Admin-Token": "FINIX-admin",
		"Authorization": "Bearer " + adminJWT,
	})
	if status != http.StatusOK {
		t.Fatalf("bias-audit status=%d payload=%v", status, payload)
	}

	// Previously shadowed user-facing routes must now be reachable with a
	// normal session token (regression guard for the mount-shadowing bug).
	status, payload = callJSON(t, http.MethodGet, ts.URL+"/v1/aiml/investments/recommend", nil, map[string]string{
		"Authorization":        "Bearer " + token,
		"X-Device-Fingerprint": "device-test-fp-1",
	})
	if status != http.StatusOK {
		t.Fatalf("aiml recommend status=%d payload=%v", status, payload)
	}
	status, payload = callJSON(t, http.MethodGet, ts.URL+"/v1/blockchain/news-whitelist", nil, map[string]string{
		"Authorization":        "Bearer " + token,
		"X-Device-Fingerprint": "device-test-fp-1",
	})
	if status != http.StatusOK {
		t.Fatalf("news-whitelist status=%d payload=%v", status, payload)
	}
}

func generateTestKeyPair(t *testing.T) (*ecdsa.PrivateKey, string, string) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key pair failed: %v", err)
	}
	pubKeyBytes := elliptic.Marshal(privKey.Curve, privKey.PublicKey.X, privKey.PublicKey.Y)
	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	keyIDBytes := make([]byte, 8)
	rand.Read(keyIDBytes)
	keyID := "key_" + base64.StdEncoding.EncodeToString(keyIDBytes)

	return privKey, pubKeyB64, keyID
}

func signChallenge(t *testing.T, privKey *ecdsa.PrivateKey, challengeB64 string) string {
	t.Helper()
	challengeBytes, err := base64.StdEncoding.DecodeString(challengeB64)
	if err != nil {
		t.Fatalf("decode challenge failed: %v", err)
	}
	digest := sha256.Sum256(challengeBytes)
	sig, err := ecdsa.SignASN1(rand.Reader, privKey, digest[:])
	if err != nil {
		t.Fatalf("sign challenge failed: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

func mustRegisterAndLogin(t *testing.T, baseURL, deviceFP, pubKeyB64, keyID string) string {
	t.Helper()
	_, payload := callJSON(t, http.MethodPost, baseURL+"/v1/auth/register", map[string]any{
		"name":                "Router Test",
		"mobile":              "9000000011",
		"deviceIdFingerprint": deviceFP,
	}, nil)
	userID := payloadString(payload, "userId")
	if userID == "" {
		t.Fatalf("register did not return userId: %v", payload)
	}
	_, _ = callJSON(t, http.MethodPost, baseURL+"/v1/auth/ekyc/verify", map[string]any{
		"userId":       userID,
		"panLast4":     "1234",
		"aadhaarLast4": "1234",
	}, nil)
	_, _ = callJSON(t, http.MethodPost, baseURL+"/v1/auth/biometric/register", map[string]any{
		"userId":       userID,
		"publicKeyB64": pubKeyB64,
		"keyId":        keyID,
	}, nil)
	return userID
}

func mustLoginToken(t *testing.T, baseURL, userID, deviceFP string, privKey *ecdsa.PrivateKey, keyID string) string {
	t.Helper()
	_, challengePayload := callJSON(t, http.MethodPost, baseURL+"/v1/auth/login/challenge", map[string]any{"userId": userID}, nil)
	challengeID := payloadString(challengePayload, "challengeId")
	challenge := payloadString(challengePayload, "challenge")
	if challenge == "" || challengeID == "" {
		t.Fatalf("missing challenge: %v", challengePayload)
	}
	signature := signChallenge(t, privKey, challenge)
	_, loginPayload := callJSON(t, http.MethodPost, baseURL+"/v1/auth/login/verify", map[string]any{
		"userId":              userID,
		"challengeId":         challengeID,
		"signature":           signature,
		"keyId":               keyID,
		"deviceIdFingerprint": deviceFP,
	}, nil)
	token := payloadString(loginPayload, "accessToken")
	if token == "" {
		t.Fatalf("login did not return token: %v", loginPayload)
	}
	return token
}

func mustFirstAccount(t *testing.T, baseURL, token, deviceFP string) string {
	t.Helper()
	status, payload := callJSON(t, http.MethodGet, baseURL+"/v1/accounts", nil, map[string]string{
		"Authorization":        "Bearer " + token,
		"X-Device-Fingerprint": deviceFP,
	})
	if status != http.StatusOK {
		t.Fatalf("accounts status=%d payload=%v", status, payload)
	}
	rows, ok := payload.([]any)
	if !ok || len(rows) == 0 {
		t.Fatalf("unexpected accounts payload: %T %v", payload, payload)
	}
	first, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected first account type: %T", rows[0])
	}
	id, _ := first["id"].(string)
	if id == "" {
		t.Fatalf("missing account id: %v", first)
	}
	return id
}

func callJSON(t *testing.T, method, url string, body any, headers map[string]string) (int, any) {
	t.Helper()
	var reqBody []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body failed: %v", err)
		}
		reqBody = encoded
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var payload any
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	return resp.StatusCode, payload
}

func payloadString(payload any, key string) string {
	asMap, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	value, _ := asMap[key].(string)
	return value
}

var _ = time.Minute
