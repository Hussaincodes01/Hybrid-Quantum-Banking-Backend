package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// RequireBiometricAboveAmount enforces a fresh biometric challenge (the
// X-Biometric-Challenge-ID + X-Biometric-Challenge headers) when the request's
// monetary amount is at or above thresholdPaise. Used for high-value investment
// orders (spec §9.4A.4: >= ₹25,000 requires biometric re-auth).
//
// It peeks at the JSON body (amountPaise / amount / orderAmountPaise) and then
// restores it so the downstream handler still reads the full body.
func RequireBiometricAboveAmount(thresholdPaise int64, maxAge time.Duration, verifier BiometricChallengeVerifier) func(http.Handler) http.Handler {
	if maxAge <= 0 {
		maxAge = 5 * time.Minute
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			amountPaise := peekAmountPaise(r)
			if amountPaise < thresholdPaise {
				next.ServeHTTP(w, r) // below threshold — no step-up needed
				return
			}

			uid, ok := UserIDFromContext(r.Context())
			if !ok || strings.TrimSpace(uid) == "" {
				writeAuthError(w, "token_invalid", "unauthorized")
				return
			}
			challengeID := strings.TrimSpace(r.Header.Get("X-Biometric-Challenge-ID"))
			signature := strings.TrimSpace(r.Header.Get("X-Biometric-Challenge"))
			if challengeID == "" || signature == "" {
				writeStepUpRequired(w, "high-value order requires biometric re-authentication")
				return
			}
			if verifier != nil {
				if err := verifier(uid, challengeID, signature, maxAge); err != nil {
					writeStepUpRequired(w, err.Error())
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeStepUpRequired emits a structured 401 the client can act on.
func writeStepUpRequired(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message, "code": "step_up_required"})
}

// peekAmountPaise reads (and restores) the body to extract an amount in paise.
// Accepts an integer *Paise field or a rupee `amount` float. Returns 0 if none.
func peekAmountPaise(r *http.Request) int64 {
	if r.Body == nil {
		return 0
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // cap at 1MB
	_ = r.Body.Close()
	// Always restore so the handler still sees the full body.
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil || len(raw) == 0 {
		return 0
	}

	var body struct {
		AmountPaise      *int64   `json:"amountPaise"`
		OrderAmountPaise *int64   `json:"orderAmountPaise"`
		UnitsAmountPaise *int64   `json:"unitsAmountPaise"`
		Amount           *float64 `json:"amount"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return 0
	}
	switch {
	case body.AmountPaise != nil:
		return *body.AmountPaise
	case body.OrderAmountPaise != nil:
		return *body.OrderAmountPaise
	case body.UnitsAmountPaise != nil:
		return *body.UnitsAmountPaise
	case body.Amount != nil:
		return int64(*body.Amount * 100)
	}
	return 0
}
