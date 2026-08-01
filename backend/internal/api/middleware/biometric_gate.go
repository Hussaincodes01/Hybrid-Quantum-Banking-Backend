package middleware

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

type BiometricChallengeVerifier func(userID, challengeID, signatureB64 string, maxAge time.Duration) error

// BiometricChallengeEnforced reports whether a signed biometric challenge is
// mandatory on the routes guarded by this middleware.
//
// Default is OFF so the demo client — which cannot mint a real hardware-signed
// challenge — can exercise flows such as security unfreeze. Set
// FINIX_REQUIRE_BIOMETRIC_CHALLENGE=true in production to enforce it. This
// mirrors the existing FINIX_REQUIRE_PQC / FINIX_PIN_ENFORCE toggles.
//
// Note the asymmetry: when the challenge headers ARE supplied they are always
// verified, even with enforcement off, so a forged signature is still rejected.
// Only the *absence* of the headers is tolerated.
func BiometricChallengeEnforced() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FINIX_REQUIRE_BIOMETRIC_CHALLENGE"))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

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
			signature := strings.TrimSpace(r.Header.Get("X-Biometric-Challenge"))
			enforced := BiometricChallengeEnforced()

			// No challenge supplied: reject when enforcement is on, otherwise let
			// the request through (demo posture) with an audit-visible warning.
			if challengeID == "" || signature == "" {
				if enforced {
					if challengeID == "" {
						http.Error(w, "biometric challenge ID is required (X-Biometric-Challenge-ID header)", http.StatusUnauthorized)
						return
					}
					http.Error(w, "biometric challenge signature is required (X-Biometric-Challenge header)", http.StatusUnauthorized)
					return
				}
				slog.Warn("biometric challenge absent; allowed because FINIX_REQUIRE_BIOMETRIC_CHALLENGE is not enabled",
					"user", uid, "path", r.URL.Path)
				next.ServeHTTP(w, r)
				return
			}

			// Headers present: always verify, regardless of the enforcement flag.
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
