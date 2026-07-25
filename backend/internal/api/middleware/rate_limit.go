package middleware

import (
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	windowStart time.Time
	count       int
}

func RateLimit(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	if maxRequests <= 0 {
		maxRequests = 60
	}
	if window <= 0 {
		window = time.Minute
	}

	var (
		mu        sync.Mutex
		buckets   = make(map[string]bucket)
		lastSweep = time.Now()
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// M-1 fix: behind a reverse proxy, r.RemoteAddr is the proxy's address, so
			// all clients would share one bucket. Prefer the first hop of
			// X-Forwarded-For (the real client), falling back to RemoteAddr.
			host := ClientIP(r)
			key := host
			now := time.Now()

			mu.Lock()
			// Lazy sweep: at most once per window, drop buckets whose window has
			// elapsed so the map can't grow unbounded with attacker-controlled IPs.
			if now.Sub(lastSweep) >= window {
				for k, b := range buckets {
					if now.Sub(b.windowStart) >= window {
						delete(buckets, k)
					}
				}
				lastSweep = now
			}
			state, ok := buckets[key]
			if !ok || now.Sub(state.windowStart) >= window {
				state = bucket{windowStart: now, count: 0}
			}
			state.count++
			buckets[key] = state
			mu.Unlock()

			if state.count > maxRequests {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
