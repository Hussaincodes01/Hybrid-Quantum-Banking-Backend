package kyc

// NSDL (Protean eGov) real KYC provider — REAL-READY STUB.
//
// This file proves the provider seam: it satisfies security.KYCProvider and is
// selectable by env (KYC_PROVIDER=nsdl), but every method returns
// ErrNotImplemented rather than faking success. Complete it by wiring:
//
//   PAN verification   POST https://<nsdl-host>/panverification/v1/verify
//                      headers: Authorization: Bearer <NSDL_KEY>
//                      body:    { "pan": "ABCDE1234F" }
//                      -> { "status": "VALID|INVALID", "name": "..." }
//
//   Aadhaar offline    Parse the UIDAI Paperless Offline e-KYC ZIP/XML with the
//                      share code, verify the UIDAI digital signature, then map
//                      the POI/POA fields into security.KYCResult.
//
//   KIN (CKYC)         POST https://<ckyc-host>/ckyc/v1/search  (uidHash+PAN)
//                      -> existing KIN, or a new one on registration.
//
// Read the base URL + key from env so no code change is needed to go live.

import (
	"context"
	"os"

	"FINIX/backend/internal/domain/security"
)

// NSDLConfig holds the real-provider credentials (from env).
type NSDLConfig struct {
	BaseURL string
	APIKey  string
}

// NSDLConfigFromEnv reads NSDL_BASE_URL / NSDL_KEY.
func NSDLConfigFromEnv() NSDLConfig {
	base := os.Getenv("NSDL_BASE_URL")
	if base == "" {
		base = "https://api.nsdl.example" // placeholder; set NSDL_BASE_URL to go live
	}
	return NSDLConfig{BaseURL: base, APIKey: os.Getenv("NSDL_KEY")}
}

// NSDL is the real KYC provider (unwired stub).
type NSDL struct {
	cfg NSDLConfig
}

var _ security.KYCProvider = (*NSDL)(nil)

// NewNSDL returns the real provider as the interface type.
func NewNSDL(cfg NSDLConfig) security.KYCProvider { return &NSDL{cfg: cfg} }

func (n *NSDL) VerifyPAN(ctx context.Context, pan string) (bool, error) {
	return false, ErrNotImplemented
}

func (n *NSDL) VerifyAadhaar(ctx context.Context, xmlB64, shareCode string) (security.KYCResult, error) {
	return security.KYCResult{}, ErrNotImplemented
}

func (n *NSDL) GenerateKIN(uidHash, panLast4 string) (string, error) {
	return "", ErrNotImplemented
}
