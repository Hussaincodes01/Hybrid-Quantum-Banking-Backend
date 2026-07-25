package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudflare/circl/kem/kyber/kyber1024"
)

// TestPQCEnforcementGatesAuthedRoutes proves that when FINIX_REQUIRE_PQC is on:
//   - an authenticated request WITHOUT a PQC session is rejected (401), and
//   - the same request WITH a session from a real Kyber-1024 (circl) handshake
//     is accepted (200).
//
// This is the server-side security control the app's X-PQC-Session relies on.
func TestPQCEnforcementGatesAuthedRoutes(t *testing.T) {
	t.Setenv("FINIX_REQUIRE_PQC", "true")

	ts := httptest.NewServer(NewRouter(nil))
	defer ts.Close()

	// Passkey login → bearer token (public endpoints, not PQC-gated).
	keyPair, pubKeyB64, keyID := generateTestKeyPair(t)
	userID := mustRegisterAndLogin(t, ts.URL, "device-pqc-fp", pubKeyB64, keyID)
	token := mustLoginToken(t, ts.URL, userID, "device-pqc-fp", keyPair, keyID)

	authHeaders := map[string]string{
		"Authorization":        "Bearer " + token,
		"X-Device-Fingerprint": "device-pqc-fp",
	}

	// Without a PQC session → must be rejected by the enforcement middleware.
	status, _ := callJSON(t, http.MethodGet, ts.URL+"/v1/dashboard", nil, authHeaders)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 without PQC session, got %d", status)
	}

	// Kyber-1024 handshake with a real ML-KEM client (circl).
	status, initBody := callJSON(t, http.MethodGet, ts.URL+"/v1/auth/pqc/init", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("pqc/init status=%d", status)
	}
	pkB64, _ := initBody.(map[string]any)["publicKey"].(string)
	pkBytes, err := base64.StdEncoding.DecodeString(pkB64)
	if err != nil || len(pkBytes) != kyber1024.PublicKeySize {
		t.Fatalf("bad server public key: err=%v len=%d", err, len(pkBytes))
	}
	pk := new(kyber1024.PublicKey)
	pk.Unpack(pkBytes)
	ct := make([]byte, kyber1024.CiphertextSize)
	ss := make([]byte, kyber1024.SharedKeySize)
	pk.EncapsulateTo(ct, ss, nil)

	status, encBody := callJSON(t, http.MethodPost, ts.URL+"/v1/auth/pqc/encapsulate",
		map[string]any{"ciphertext": base64.StdEncoding.EncodeToString(ct)}, nil)
	if status != http.StatusOK {
		t.Fatalf("pqc/encapsulate status=%d", status)
	}
	sessionID, _ := encBody.(map[string]any)["sessionId"].(string)
	if sessionID == "" {
		t.Fatal("no sessionId returned from encapsulate")
	}

	// With a valid PQC session → request is accepted.
	pqcHeaders := map[string]string{
		"Authorization":        "Bearer " + token,
		"X-Device-Fingerprint": "device-pqc-fp",
		"X-PQC-Session":        sessionID,
	}
	status, _ = callJSON(t, http.MethodGet, ts.URL+"/v1/dashboard", nil, pqcHeaders)
	if status != http.StatusOK {
		t.Fatalf("expected 200 with PQC session, got %d", status)
	}
}
