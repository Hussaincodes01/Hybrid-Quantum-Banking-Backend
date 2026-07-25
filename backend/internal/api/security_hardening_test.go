package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRegisterRejectsInvalidMobile proves strict field validation rejects a
// malformed mobile number before it reaches the domain layer.
func TestRegisterRejectsInvalidMobile(t *testing.T) {
	ts := httptest.NewServer(NewRouter(nil))
	defer ts.Close()

	status, _ := callJSON(t, http.MethodPost, ts.URL+"/v1/auth/register", map[string]any{
		"name": "Bad Mobile", "mobile": "12345", "email": "x@finix.app",
		"deviceIdFingerprint": "dev-1", "deviceType": "android", "appVersion": "1.0.0",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid mobile, got %d", status)
	}

	// A valid Indian mobile is accepted.
	status, _ = callJSON(t, http.MethodPost, ts.URL+"/v1/auth/register", map[string]any{
		"name": "Good Mobile", "mobile": "+919876500011", "email": "y@finix.app",
		"deviceIdFingerprint": "dev-1", "deviceType": "android", "appVersion": "1.0.0",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for valid mobile, got %d", status)
	}
}

// TestEKYCRejectsBadLast4 proves the Aadhaar/PAN last-4 format is enforced.
func TestEKYCRejectsBadLast4(t *testing.T) {
	ts := httptest.NewServer(NewRouter(nil))
	defer ts.Close()

	status, _ := callJSON(t, http.MethodPost, ts.URL+"/v1/auth/ekyc/verify", map[string]any{
		"userId": "usr_x", "panLast4": "12", "aadhaarLast4": "5678",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad panLast4, got %d", status)
	}
}

// TestAuthBruteForceLimiter proves credential endpoints are rate-limited far
// below the global limit (20/min shared across auth endpoints per IP).
func TestAuthBruteForceLimiter(t *testing.T) {
	ts := httptest.NewServer(NewRouter(nil))
	defer ts.Close()

	got429 := false
	for i := 0; i < 40; i++ {
		status, _ := callJSON(t, http.MethodPost, ts.URL+"/v1/auth/login/verify", map[string]any{
			"userId": "usr_none", "challengeId": "x", "signature": "x", "keyId": "x",
			"deviceIdFingerprint": "dev",
		}, nil)
		if status == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("expected 429 from the auth brute-force limiter within 40 attempts")
	}
}
