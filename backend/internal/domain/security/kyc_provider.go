package security

import "context"

// KYCProvider is a swappable eKYC provider (PAN + Aadhaar verification, KIN
// generation). The default is a local mock (internal/infra/kyc); a real
// provider (NSDL/UIDAI) can be selected by env later without changing callers.
//
// Provider-agnostic: no NSDL/UIDAI-specific fields. Existing helpers in this
// package (VerifyAadhaarOfflineXML, GenerateKIN, VerifyPAN) remain available for
// mocks and real providers to build on.
type KYCProvider interface {
	// VerifyPAN checks a PAN with the authority. verified=false (nil err) means
	// the PAN is well-formed but not on record; err is a transport/authority
	// failure.
	VerifyPAN(ctx context.Context, pan string) (verified bool, err error)

	// VerifyAadhaar validates an offline Aadhaar e-KYC XML (base64) against its
	// share code and returns the parsed identity.
	VerifyAadhaar(ctx context.Context, xmlB64, shareCode string) (KYCResult, error)

	// GenerateKIN derives the CKYC Identifier Number from the Aadhaar UID hash
	// and PAN last-4.
	GenerateKIN(uidHash, panLast4 string) (string, error)
}

// KYCResult is the provider-agnostic outcome of an Aadhaar verification.
type KYCResult struct {
	Verified    bool   `json:"verified"`
	Name        string `json:"name"`
	DOB         string `json:"dob"`
	Gender      string `json:"gender"`
	Address     string `json:"address"`
	ReferenceID string `json:"referenceId"`
}
