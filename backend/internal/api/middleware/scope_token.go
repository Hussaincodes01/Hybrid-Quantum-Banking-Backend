package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func RequireScopedToken(headerName, expectedToken string) func(http.Handler) http.Handler {
	headerName = strings.TrimSpace(headerName)
	if headerName == "" {
		headerName = "X-Internal-Token"
	}
	expectedToken = strings.TrimSpace(expectedToken)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// M-6 fix: fail closed. Previously an empty expected token fell back to
			// the hardcoded "FINIX-dev", which a misconfigured chain would accept as
			// a valid admin token. Now an unset expected token denies all requests.
			if expectedToken == "" {
				http.Error(w, "scope token not configured", http.StatusServiceUnavailable)
				return
			}
			provided := strings.TrimSpace(r.Header.Get(headerName))
			if provided == "" {
				http.Error(w, "missing scope token", http.StatusUnauthorized)
				return
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(expectedToken)) != 1 {
				http.Error(w, "invalid scope token", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
