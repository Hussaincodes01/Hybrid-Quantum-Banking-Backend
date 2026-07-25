package bank

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

// ErrNotImplemented is returned by real-provider stubs that are not yet wired.
// It is deliberately distinct so callers can tell a missing integration from a
// genuine "not found" or a transport failure — a stub never fakes success.
var ErrNotImplemented = errors.New("bank provider not implemented: real integration not wired")

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(b)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
