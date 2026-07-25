package middleware

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// DistributedRateLimiter provides Redis-backed rate limiting for multi-instance deployments.
type DistributedRateLimiter struct {
	client    *redis.Client
	maxReq    int
	window    time.Duration
	keyPrefix string
}

// NewDistributedRateLimiter creates a new Redis-backed rate limiter.
func NewDistributedRateLimiter(client *redis.Client, maxRequests int, window time.Duration, keyPrefix string) *DistributedRateLimiter {
	if maxRequests <= 0 {
		maxRequests = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	if keyPrefix == "" {
		keyPrefix = "ratelimit"
	}
	return &DistributedRateLimiter{
		client:     client,
		maxReq:     maxRequests,
		window:     window,
		keyPrefix:  keyPrefix,
	}
}

// Middleware returns a rate limiting middleware using Redis.
func (rl *DistributedRateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rl.keyPrefix + ":" + clientIP(r)
			
			ctx := context.Background()
			
			// Use Redis INCR with TTL for sliding window
			current, err := rl.client.Incr(ctx, key).Result()
			if err != nil {
				// On Redis error, allow request but log error (fail-open for availability)
				next.ServeHTTP(w, r)
				return
			}
			
			if current == 1 {
				// First request in window, set expiry
				rl.client.Expire(ctx, key, rl.window)
			}
			
			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.maxReq))
			remaining := rl.maxReq - int(current)
			if remaining < 0 {
				remaining = 0
			}
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(rl.window).Unix(), 10))
			
			if int(current) > rl.maxReq {
				ttl, _ := rl.client.TTL(ctx, key).Result()
				w.Header().Set("Retry-After", strconv.FormatInt(int64(ttl.Seconds()), 10))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP returns the best-effort real client IP.
func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			xff = xff[:idx]
		}
		if ip := strings.TrimSpace(xff); ip != "" {
			return ip
		}
	}
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}