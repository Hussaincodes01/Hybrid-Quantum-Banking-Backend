package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"sync"
)

// wildcardDomain holds pins that apply to every host (loaded from CERT_PINS).
const wildcardDomain = "*"

// CertificatePinningManager manages certificate pin validation per spec §3.6:
// "Network requests are pinned to specific certificate fingerprints — man-in-the-middle
// attacks using forged certificates will be rejected."
//
// In a Flutter/mobile client, cert pinning is done at the HTTP client level.
// On the server side, we provide:
// 1. A registry of expected certificate fingerprints per domain
// 2. A middleware that validates the client's presented certificate fingerprint
// 3. An endpoint for clients to verify they're talking to the right server

type CertificatePinningManager struct {
	mu   sync.RWMutex
	pins map[string][]string // domain → list of valid SHA-256 fingerprint hashes
	// enforce (H-4): when true and pins are configured, a request that presents NO
	// verifiable certificate fingerprint is rejected rather than skipped — closing
	// the "omit the header to bypass pinning" gap. Requires that this process
	// actually receives the client cert (mTLS) or a trusted fingerprint header.
	enforce bool
}

func NewCertificatePinningManager() *CertificatePinningManager {
	return &CertificatePinningManager{pins: make(map[string][]string)}
}

// NewCertificatePinningManagerFromEnv loads the pin set from CERT_PINS
// (comma-separated SHA-256 fingerprints). The pins apply to every host so a
// forged certificate presenting an unlisted fingerprint is rejected. With
// CERT_PINS unset the manager holds no pins and pinning is effectively disabled
// (open policy) — the real pin is also enforced client-side.
func NewCertificatePinningManagerFromEnv() *CertificatePinningManager {
	cpm := NewCertificatePinningManager()
	for _, raw := range strings.Split(os.Getenv("CERT_PINS"), ",") {
		pin := strings.TrimSpace(raw)
		if pin != "" {
			cpm.pins[wildcardDomain] = append(cpm.pins[wildcardDomain], pin)
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FINIX_PIN_ENFORCE"))) {
	case "1", "true", "yes", "on":
		cpm.enforce = true
	}
	return cpm
}

// EnforceStrict reports whether missing fingerprints are rejected (H-4).
func (cpm *CertificatePinningManager) EnforceStrict() bool { return cpm.enforce }

// PinCount returns the number of configured pins (across all hosts).
func (cpm *CertificatePinningManager) PinCount() int {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()
	n := 0
	for _, pins := range cpm.pins {
		n += len(pins)
	}
	return n
}

// Enabled reports whether any pins are configured (pinning is enforced).
func (cpm *CertificatePinningManager) Enabled() bool { return cpm.PinCount() > 0 }

// AddPin adds a certificate pin for a domain.
func (cpm *CertificatePinningManager) AddPin(domain, fingerprint string) {
	cpm.mu.Lock()
	defer cpm.mu.Unlock()
	cpm.pins[domain] = append(cpm.pins[domain], fingerprint)
}

// RemovePin removes a certificate pin for a domain.
func (cpm *CertificatePinningManager) RemovePin(domain, fingerprint string) {
	cpm.mu.Lock()
	defer cpm.mu.Unlock()
	pins := cpm.pins[domain]
	for i, pin := range pins {
		if subtle.ConstantTimeCompare([]byte(pin), []byte(fingerprint)) == 1 {
			cpm.pins[domain] = append(pins[:i], pins[i+1:]...)
			return
		}
	}
}

// ValidatePin checks if a certificate fingerprint matches a pinned cert for a domain.
func (cpm *CertificatePinningManager) ValidatePin(domain, fingerprint string) bool {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()

	// Combine host-specific pins with the wildcard set loaded from CERT_PINS.
	pins := append(append([]string(nil), cpm.pins[domain]...), cpm.pins[wildcardDomain]...)
	if len(pins) == 0 {
		// No pins registered — allow (open pinning policy).
		return true
	}
	for _, pin := range pins {
		if subtle.ConstantTimeCompare([]byte(pin), []byte(fingerprint)) == 1 {
			return true
		}
	}
	return false
}

// GetPins returns all pins for a domain (used by the pin verification endpoint).
func (cpm *CertificatePinningManager) GetPins(domain string) []string {
	cpm.mu.RLock()
	defer cpm.mu.RUnlock()
	return append([]string(nil), cpm.pins[domain]...)
}

// PinVerificationMiddleware validates the client's certificate fingerprint against
// the pinned set. H-4 fix: it prefers the ACTUAL client certificate from the TLS
// handshake (r.TLS.PeerCertificates — genuine mTLS) over the client-supplied
// X-Cert-Fingerprint header, which an attacker can forge or omit. The header is
// used only as a fallback when this process does not terminate TLS with client
// certs (e.g. behind a TLS-terminating proxy). When pins are configured and
// FINIX_PIN_ENFORCE is set, a request presenting NO verifiable fingerprint is
// rejected instead of silently skipped, closing the omit-the-header bypass.
func PinVerificationMiddleware(cpm *CertificatePinningManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fingerprint := ""
			if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
				// Derive the fingerprint server-side from the presented client cert —
				// this cannot be spoofed by a header.
				sum := sha256.Sum256(r.TLS.PeerCertificates[0].Raw)
				fingerprint = hex.EncodeToString(sum[:])
			} else {
				fingerprint = strings.TrimSpace(r.Header.Get("X-Cert-Fingerprint"))
			}

			if fingerprint == "" {
				if cpm.Enabled() && cpm.enforce {
					http.Error(w, "client certificate required for pinned endpoint", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r) // open policy: nothing to verify
				return
			}

			if !cpm.ValidatePin(r.Host, fingerprint) {
				http.Error(w, "certificate pin validation failed — possible MITM attack", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
