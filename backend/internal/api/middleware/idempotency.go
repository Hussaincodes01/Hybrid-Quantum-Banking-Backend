package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RequireIdempotency enforces an Idempotency-Key on mutating requests to the given
// path prefixes AND (L-3 fix) deduplicates on the request body: a repeat of the
// same key with the SAME body is a safe client retry, but the same key with a
// DIFFERENT body is a contract violation / tampered replay and is rejected 409.
// Response-level idempotency (returning the original result for a retry) is handled
// in the service layer; this guards the key↔payload binding at the edge.
func RequireIdempotency(pathPrefixes ...string) func(http.Handler) http.Handler {
	const (
		ttl        = 24 * time.Hour
		maxBody    = 1 << 20 // 1 MiB — bodies are already capped by http.MaxBytesReader
		maxEntries = 50000
	)
	type entry struct {
		bodyHash string
		expires  time.Time
	}
	var (
		mu        sync.Mutex
		seen      = make(map[string]entry)
		lastSweep = time.Now()
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := strings.ToUpper(r.Method)
			if method != http.MethodPost && method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			requiresKey := false
			for _, prefix := range pathPrefixes {
				if strings.HasPrefix(r.URL.Path, prefix) {
					requiresKey = true
					break
				}
			}
			if !requiresKey {
				next.ServeHTTP(w, r)
				return
			}

			key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
			if key == "" {
				http.Error(w, "idempotency key required", http.StatusBadRequest)
				return
			}

			// Buffer and restore the body so the handler still reads it in full.
			var bodyBytes []byte
			if r.Body != nil {
				bodyBytes, _ = io.ReadAll(io.LimitReader(r.Body, maxBody))
				_ = r.Body.Close()
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
			sum := sha256.Sum256(bodyBytes)
			hash := hex.EncodeToString(sum[:])

			// Scope the key to the authenticated user (Auth runs before this) so one
			// user's key can never collide with another's.
			scope := "anon"
			if uid, ok := UserIDFromContext(r.Context()); ok {
				scope = uid
			}
			storeKey := scope + "|" + r.URL.Path + "|" + key

			now := time.Now()
			mu.Lock()
			if now.Sub(lastSweep) >= time.Hour || len(seen) > maxEntries {
				for k, e := range seen {
					if now.After(e.expires) {
						delete(seen, k)
					}
				}
				lastSweep = now
			}
			if prev, ok := seen[storeKey]; ok && now.Before(prev.expires) {
				if prev.bodyHash != hash {
					mu.Unlock()
					http.Error(w, "idempotency key reused with a different request body", http.StatusConflict)
					return
				}
				// same key + same body → legitimate retry; fall through.
			} else {
				seen[storeKey] = entry{bodyHash: hash, expires: now.Add(ttl)}
			}
			mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}
