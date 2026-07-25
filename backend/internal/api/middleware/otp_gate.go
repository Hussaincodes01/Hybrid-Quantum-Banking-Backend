package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type OTPVerifier func(userID, otp string) error

func RequireOTPVerification(verifier OTPVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, ok := UserIDFromContext(r.Context())
			if !ok || strings.TrimSpace(uid) == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			if r.Body == nil {
				http.Error(w, "request body is required", http.StatusBadRequest)
				return
			}

			raw, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read request body", http.StatusBadRequest)
				return
			}
			r.Body.Close()
			r.Body = io.NopCloser(bytes.NewBuffer(raw))

			var payload map[string]any
			if len(bytes.TrimSpace(raw)) > 0 {
				if err := json.Unmarshal(raw, &payload); err != nil {
					http.Error(w, "invalid json body", http.StatusBadRequest)
					return
				}
			}

			otp := strings.TrimSpace(extractOTPFromPayload(payload))
			if otp == "" {
				http.Error(w, "otp token is required", http.StatusBadRequest)
				return
			}

			if verifier != nil {
				if err := verifier(uid, otp); err != nil {
					http.Error(w, err.Error(), http.StatusUnauthorized)
					return
				}
			}

			r.Body = io.NopCloser(bytes.NewBuffer(raw))
			next.ServeHTTP(w, r)
		})
	}
}

func extractOTPFromPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	keys := []string{"otp", "otpToken", "otp_token"}
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if otp, ok := value.(string); ok {
				return otp
			}
		}
	}
	return ""
}
