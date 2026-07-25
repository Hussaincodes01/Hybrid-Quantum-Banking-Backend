package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsProbe(t *testing.T, allow []string, origin string) *httptest.ResponseRecorder {
	t.Helper()
	h := corsMiddleware(allow)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// MF5: an allow-listed origin is reflected AND credentialed.
func TestCORSExplicitOriginGetsCredentials(t *testing.T) {
	rec := corsProbe(t, []string{"https://app.finix.example"}, "https://app.finix.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.finix.example" {
		t.Fatalf("Allow-Origin = %q, want the exact origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials = %q, want true for an allow-listed origin", got)
	}
}

// MF5: an origin not on the list is rejected.
func TestCORSUnlistedOriginRejected(t *testing.T) {
	rec := corsProbe(t, []string{"https://app.finix.example"}, "https://evil.example")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unlisted origin: code=%d, want 403", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("rejected origin must not be reflected, got %q", got)
	}
}

// MF5: a wildcard policy is allowed but is NEVER credentialed, and "*" is never
// emitted together with Allow-Credentials:true.
func TestCORSWildcardNeverCredentialed(t *testing.T) {
	rec := corsProbe(t, []string{"*"}, "https://anything.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("wildcard Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("wildcard must NOT set Allow-Credentials, got %q", got)
	}
}
