package api

import "testing"

// A CORS allow-list bug hands an attacker's page authenticated access, so the
// negative cases matter more than the positive one.
func TestOriginMatchesPortWildcard(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		origin  string
		want    bool
	}{
		// Intended use: any port of an already-trusted scheme+host.
		{"localhost any port", "http://localhost:*", "http://localhost:5599", true},
		{"localhost other port", "http://localhost:*", "http://localhost:8080", true},
		{"loopback ip", "http://127.0.0.1:*", "http://127.0.0.1:3000", true},
		{"case insensitive host", "http://LocalHost:*", "http://localhost:41", true},

		// Must not widen to a different host.
		{"suffix host attack", "http://localhost:*", "http://localhost.evil.com:80", false},
		{"prefix host attack", "http://localhost:*", "http://notlocalhost:80", false},
		{"subdomain attack", "http://localhost:*", "http://evil.localhost:80", false},

		// Must not cross schemes.
		{"scheme mismatch", "http://localhost:*", "https://localhost:5599", false},

		// The remainder must be exactly ":<digits>".
		{"no port", "http://localhost:*", "http://localhost", false},
		{"empty port", "http://localhost:*", "http://localhost:", false},
		{"port with path", "http://localhost:*", "http://localhost:80/admin", false},
		{"non-numeric port", "http://localhost:*", "http://localhost:80a", false},
		{"userinfo smuggling", "http://localhost:*", "http://localhost:80@evil.com", false},

		// A pattern without the marker must never match through this path.
		{"exact pattern ignored here", "http://localhost:5599", "http://localhost:5599", false},
		{"bare star ignored here", "*", "http://anything", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := originMatchesPortWildcard(tc.pattern, tc.origin); got != tc.want {
				t.Errorf("originMatchesPortWildcard(%q, %q) = %v, want %v",
					tc.pattern, tc.origin, got, tc.want)
			}
		})
	}
}
