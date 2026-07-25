package middleware

import (
	"context"
	"net/http"
	"strings"
)

type PQCSessionValidator func(sessionID string) bool

type pqcContextKey string

const pqcSessionIDKey pqcContextKey = "pqcSessionID"

func RequirePQCSession(validator PQCSessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionID := strings.TrimSpace(r.Header.Get("X-PQC-Session"))
			if sessionID == "" {
				http.Error(w, "PQC session is required (X-PQC-Session header)", http.StatusUnauthorized)
				return
			}
			if validator != nil {
				if !validator(sessionID) {
					http.Error(w, "invalid or expired PQC session", http.StatusUnauthorized)
					return
				}
			}
			ctx := context.WithValue(r.Context(), pqcSessionIDKey, sessionID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func PQCSessionsFromContext(ctx context.Context) (string, bool) {
	value := ctx.Value(pqcSessionIDKey)
	if value == nil {
		return "", false
	}
	sessionID, ok := value.(string)
	return sessionID, ok && sessionID != ""
}
