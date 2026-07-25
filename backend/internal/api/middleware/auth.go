package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey string

const userIDContextKey contextKey = "userID"

type Authenticator func(token, deviceFingerprint string) (userID string, ok bool)

type RoleFetcher func(userID string) string

// TokenClaims is the identity extracted from a verified JWT.
type TokenClaims struct {
	UserID   string
	Role     string
	DeviceFP string
}

// JWTVerify verifies a bearer JWT against the request's device fingerprint.
// On success it returns claims; on failure a structured code:
// "token_expired", "device_mismatch", "token_revoked" or "token_invalid".
// A nil verifier (unit tests) skips straight to the legacy authenticator.
type JWTVerify func(token, deviceFP string) (claims *TokenClaims, code string, ok bool)

// writeAuthError emits a structured 401 the Flutter interceptor can branch on
// ({"error":..,"code":..}) — token_expired triggers a silent re-auth,
// device_mismatch forces re-login.
func writeAuthError(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message, "code": code})
}

// Auth verifies the bearer token. A JWT (verify) is checked first with typed
// failure codes; anything that isn't a valid JWT falls back to the legacy
// opaque-token authenticator so existing/OAuth sessions keep working.
func Auth(verify JWTVerify, authenticator Authenticator, roleFetcher RoleFetcher) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := strings.TrimSpace(r.Header.Get("Authorization"))
			if header == "" || !strings.HasPrefix(strings.ToLower(header), "bearer ") {
				writeAuthError(w, "missing_token", "missing or invalid authorization header")
				return
			}
			token := strings.TrimSpace(header[7:])
			if token == "" {
				writeAuthError(w, "missing_token", "missing bearer token")
				return
			}
			deviceFingerprint := strings.TrimSpace(r.Header.Get("X-Device-Fingerprint"))

			// 1) JWT path (structured errors).
			if verify != nil {
				claims, code, ok := verify(token, deviceFingerprint)
				if ok && claims != nil {
					ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
					role := claims.Role
					if role == "" && roleFetcher != nil {
						role = roleFetcher(claims.UserID)
					}
					ctx = context.WithValue(ctx, roleContextKey, ParseRole(role))
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				switch code {
				case "token_expired":
					writeAuthError(w, "token_expired", "session expired; please re-authenticate")
					return
				case "device_mismatch":
					writeAuthError(w, "device_mismatch", "device fingerprint does not match this session")
					return
				case "token_revoked":
					writeAuthError(w, "token_revoked", "session has been revoked")
					return
				}
				// code == "token_invalid" (not a JWT) -> fall through to legacy.
			}

			// 2) Legacy opaque-token path.
			userID, ok := authenticator(token, deviceFingerprint)
			if !ok || userID == "" {
				writeAuthError(w, "token_invalid", "invalid session token or device mismatch")
				return
			}
			ctx := context.WithValue(r.Context(), userIDContextKey, userID)
			if roleFetcher != nil {
				ctx = context.WithValue(ctx, roleContextKey, ParseRole(roleFetcher(userID)))
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	value := ctx.Value(userIDContextKey)
	if value == nil {
		return "", false
	}
	uid, ok := value.(string)
	return uid, ok && uid != ""
}
