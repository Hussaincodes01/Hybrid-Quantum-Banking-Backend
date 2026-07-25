package middleware

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the best-effort real client IP.
// It prefers the first hop of X-Forwarded-For (set by the trusted reverse proxy),
// then X-Real-IP, then falls back to the connection's RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		// First comma-separated entry is the original client.
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