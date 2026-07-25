// Package kyc provides eKYC providers for the FINIX platform.
//
// MockKYC is the DEFAULT provider. It performs a real provider-shaped round-trip
// (format checks, offline-XML parse, KIN derivation) so the seam behaves like a
// real integration, but returns verified without contacting a true authority.
// A real provider (nsdl.go) can be swapped in by env with no call-site changes.
package kyc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"FINIX/backend/internal/domain/security"
)

// ErrNotImplemented marks a real provider that is not yet wired.
var ErrNotImplemented = errors.New("kyc provider not implemented: real integration not wired")

// MockKYC is an offline KYCProvider.
type MockKYC struct {
	// latency simulates the provider round-trip for demo realism.
	latency time.Duration
}

var _ security.KYCProvider = (*MockKYC)(nil)

// NewMockKYC returns the default mock as the interface type.
func NewMockKYC() security.KYCProvider { return &MockKYC{} }

func (m *MockKYC) simulate(ctx context.Context) error {
	if m.latency > 0 {
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// VerifyPAN validates the PAN format via the security package, then returns
// verified after a simulated authority call.
func (m *MockKYC) VerifyPAN(ctx context.Context, pan string) (bool, error) {
	if err := m.simulate(ctx); err != nil {
		return false, err
	}
	pan = strings.ToUpper(strings.TrimSpace(pan))
	if err := security.VerifyPAN(pan); err != nil {
		return false, err
	}
	// A real provider would confirm the PAN is on the NSDL roll; the mock
	// treats a well-formed PAN as verified.
	return true, nil
}

// VerifyAadhaar parses the offline e-KYC XML and returns the identity fields.
func (m *MockKYC) VerifyAadhaar(ctx context.Context, xmlB64, shareCode string) (security.KYCResult, error) {
	if err := m.simulate(ctx); err != nil {
		return security.KYCResult{}, err
	}
	if strings.TrimSpace(xmlB64) == "" {
		return security.KYCResult{}, fmt.Errorf("aadhaar xml is required")
	}
	parsed, result, err := security.VerifyAadhaarOfflineXML(xmlB64, shareCode)
	if err != nil {
		return security.KYCResult{}, err
	}
	// MockKYC is the DEFAULT, explicitly-fake provider (see package doc): it treats
	// a well-formed, parseable offline-XML as verified FOR THE DEMO. This is the
	// mock's own decision — it does NOT come from VerifyAadhaarOfflineXML, which
	// (correctly) reports Verified:false because no UIDAI signature check is done.
	// Swap in a real KYCProvider (nsdl.go) to get genuine verification.
	out := security.KYCResult{Verified: true}
	if result != nil {
		out.Name = result.FullName
		out.DOB = result.DateOfBirth
		out.Gender = result.Gender
		out.Address = result.AddressSummary
		out.ReferenceID = result.UIDHash
	}
	if parsed != nil { // fall back to raw XML fields if the summary was empty
		if out.Name == "" {
			out.Name = parsed.Name
		}
		if out.DOB == "" {
			out.DOB = parsed.DOB
		}
		if out.Gender == "" {
			out.Gender = parsed.Gender
		}
		if out.Address == "" {
			out.Address = strings.TrimSpace(strings.Join(nonEmpty(
				parsed.AddressLine, parsed.Locality, parsed.District, parsed.State, parsed.Pincode), ", "))
		}
	}
	if out.ReferenceID == "" {
		out.ReferenceID = fmt.Sprintf("MOCKKYC-%X", time.Now().UnixNano())
	}
	return out, nil
}

func nonEmpty(vals ...string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

// GenerateKIN derives the CKYC identifier via the security package.
func (m *MockKYC) GenerateKIN(uidHash, panLast4 string) (string, error) {
	kin := security.GenerateKIN(uidHash, panLast4)
	if kin == "" {
		return "", fmt.Errorf("could not generate KIN")
	}
	return kin, nil
}
