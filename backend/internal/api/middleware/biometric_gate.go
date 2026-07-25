package middleware

import (
	"net/http"
	"strings"
	"time"
)

type BiometricChallengeVerifier func(userID, challengeID, signatureB64 string, maxAge time.Duration) error

func RequireBiometricChallenge(maxAge time.Duration, verifier BiometricChallengeVerifier) func(http.Handler) http.Handler {
	if maxAge <= 0 {
		maxAge = 5 * time.Minute
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, ok := UserIDFromContext(r.Context())
			if !ok || strings.TrimSpace(uid) == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			challengeID := strings.TrimSpace(r.Header.Get("X-Biometric-Challenge-ID"))
			if challengeID == "" {
				http.Error(w, "biometric challenge ID is required (X-Biometric-Challenge-ID header)", http.StatusUnauthorized)
				return
			}
			signature := strings.TrimSpace(r.Header.Get("X-Biometric-Challenge"))
			if signature == "" {
				http.Error(w, "biometric challenge signature is required (X-Biometric-Challenge header)", http.StatusUnauthorized)
				return
			}
			if verifier != nil {
				if err := verifier(uid, challengeID, signature, maxAge); err != nil {
					http.Error(w, err.Error(), http.StatusUnauthorized)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
