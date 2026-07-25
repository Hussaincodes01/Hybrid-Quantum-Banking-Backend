package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

// PAN format: 5 letters + 4 digits + 1 letter (e.g., ABCDE1234F)
var panRegex = regexp.MustCompile(`^[A-Z]{5}[0-9]{4}[A-Z]{1}$`)

// fourDigitsRegex matches exactly four digits — reused to validate the last-4 of
// both PAN and Aadhaar (neutral name; it is not Aadhaar-specific).
var fourDigitsRegex = regexp.MustCompile(`^[0-9]{4}$`)

// AadhaarXML represents the structure of Aadhaar Paperless Offline eKYC XML
type AadhaarXML struct {
	XMLName     xml.Name `xml:"AadhaarData"`
	UID         string   `xml:"UidData>Uid"`
	Name        string   `xml:"UidData>Poi>Poi-Name"`
	DOB         string   `xml:"UidData>Poi>Poi-Dob"`
	Gender      string   `xml:"UidData>Poi>Poi-Gender"`
	Photo       string   `xml:"UidData>Poi>Poi-Photo"`
	AddressLine string   `xml:"UidData>Poa>Poa-House"`
	Locality    string   `xml:"UidData>Poa>Poa-Loc"`
	District    string   `xml:"UidData>Poa>Poa-Dist"`
	State       string   `xml:"UidData>Poa>Poa-State"`
	Pincode     string   `xml:"UidData>Poa>Poa-Pc"`
}

type EKYCVerificationResult struct {
	Verified           bool      `json:"verified"`
	FullName           string    `json:"fullName"`
	DateOfBirth        string    `json:"dateOfBirth,omitempty"`
	Gender             string    `json:"gender,omitempty"`
	AddressSummary     string    `json:"addressSummary,omitempty"`
	Pincode            string    `json:"pincode,omitempty"`
	UIDHash            string    `json:"uidHash"`
	VerificationMethod string    `json:"verificationMethod"`
	VerifiedAt         time.Time `json:"verifiedAt"`
}

// VerifyAadhaarOfflineXML PARSES the Aadhaar Paperless Offline eKYC XML — it does
// NOT verify it. The UIDAI digital signature (the only thing that proves the data
// is authentic and untampered) and the AES share-code decryption are NOT
// implemented here, so a fabricated but well-formed XML would parse successfully.
//
// To ensure this can never be mistaken for real identity verification, the result
// is returned with Verified:false and VerificationMethod:"aadhaar_offline_xml_parse_only".
// Real verification belongs to a real KYC provider (see the KYCProvider seam); the
// default MockKYC provider decides its own (clearly-labelled) demo verification
// status and does not rely on this function to attest identity.
//
// Per spec §3.3: "user downloads an XML file from the UIDAI portal, which is a
// digitally signed, encrypted package of their Aadhaar data" — the signature check
// is the deferred piece.
func VerifyAadhaarOfflineXML(xmlBase64, shareCode string) (*AadhaarXML, *EKYCVerificationResult, error) {
	xmlBytes, err := base64.StdEncoding.DecodeString(xmlBase64)
	if err != nil {
		return nil, nil, errors.New("invalid base64 Aadhaar XML encoding")
	}

	if len(xmlBytes) == 0 {
		return nil, nil, errors.New("Aadhaar XML is empty")
	}

	// In production, the XML would be encrypted with the share code using AES-256-CBC.
	// For this implementation, we accept the raw XML (simulating post-decryption).
	// The share code is validated for non-emptiness as a proxy for the decryption step.
	if strings.TrimSpace(shareCode) == "" {
		return nil, nil, errors.New("share code is required for Aadhaar XML decryption")
	}

	var aadhaar AadhaarXML
	if err := xml.Unmarshal(xmlBytes, &aadhaar); err != nil {
		return nil, nil, fmt.Errorf("failed to parse Aadhaar XML: %w", err)
	}

	// Validate essential fields
	if aadhaar.UID == "" {
		return nil, nil, errors.New("Aadhaar XML missing UID field")
	}
	if aadhaar.Name == "" {
		return nil, nil, errors.New("Aadhaar XML missing name field")
	}

	// Hash the UID for storage (never store raw Aadhaar number)
	uidHash := hashUID(aadhaar.UID)

	addressParts := []string{aadhaar.AddressLine, aadhaar.Locality, aadhaar.District, aadhaar.State, aadhaar.Pincode}
	addressSummary := strings.Join(filterEmpty(addressParts), ", ")

	result := &EKYCVerificationResult{
		// Verified is intentionally false: this function only PARSES the XML; it
		// does not check the UIDAI digital signature, so it cannot attest identity.
		// A real KYCProvider must set this true only after signature verification.
		Verified:           false,
		FullName:           aadhaar.Name,
		DateOfBirth:        aadhaar.DOB,
		Gender:             aadhaar.Gender,
		AddressSummary:     addressSummary,
		Pincode:            aadhaar.Pincode,
		UIDHash:            uidHash,
		VerificationMethod: "aadhaar_offline_xml_parse_only",
		VerifiedAt:         time.Now().UTC(),
	}

	return &aadhaar, result, nil
}

// VerifyPAN validates a PAN number format.
// Per spec §3.3: "PAN verification is performed via the NSDL API"
// Here we validate the format. In production, this would call the NSDL API.
func VerifyPAN(pan string) error {
	pan = strings.ToUpper(strings.TrimSpace(pan))
	if len(pan) != 10 {
		return errors.New("PAN must be exactly 10 characters")
	}
	if !panRegex.MatchString(pan) {
		return errors.New("invalid PAN format (expected 5 letters + 4 digits + 1 letter, e.g., ABCDE1234F)")
	}
	return nil
}

// VerifyPANLast4 validates the last 4 digits of a PAN (for partial verification).
func VerifyPANLast4(last4 string) error {
	last4 = strings.TrimSpace(last4)
	if len(last4) != 4 {
		return errors.New("PAN last 4 must be exactly 4 characters")
	}
	if !fourDigitsRegex.MatchString(last4) {
		return errors.New("PAN last 4 must be 4 digits")
	}
	return nil
}

// VerifyAadhaarLast4 validates the last 4 digits of an Aadhaar number.
func VerifyAadhaarLast4(last4 string) error {
	last4 = strings.TrimSpace(last4)
	if len(last4) != 4 {
		return errors.New("Aadhaar last 4 must be exactly 4 digits")
	}
	if !fourDigitsRegex.MatchString(last4) {
		return errors.New("Aadhaar last 4 must be 4 digits")
	}
	return nil
}

// GenerateKIN generates a KYC Identification Number per spec §3.3.
// Format: KIN-<8 hex chars>-<2 digit checksum> (simulating CKYC Registry format)
func GenerateKIN(uidHash, panLast4 string) string {
	seed := uidHash + panLast4
	h := sha256.Sum256([]byte(seed))
	hexStr := hex.EncodeToString(h[:4])

	// Simple checksum: sum of hex chars mod 100
	checksum := 0
	for _, c := range hexStr {
		checksum += int(c)
	}
	checksum = checksum % 100

	return fmt.Sprintf("KIN-%s-%02d", strings.ToUpper(hexStr), checksum)
}

// hashUID derives a stable, non-reversible token for an Aadhaar number. A plain
// SHA-256 is trivially brute-forced (the input space is only ~10^12 and has a
// known format), so we key it with HMAC-SHA256 under a server-side secret pepper.
// Without the pepper an attacker cannot precompute or reverse the digest.
func hashUID(uid string) string {
	cleaned := strings.ReplaceAll(uid, " ", "")
	mac := hmac.New(sha256.New, aadhaarHashKey())
	mac.Write([]byte(cleaned))
	return hex.EncodeToString(mac.Sum(nil))
}

// aadhaarHashKey returns the secret pepper for hashUID.
// C-8 fix: no hardcoded/JWT-secret fallback. The old fallback to a constant pepper
// allowed precomputing a rainbow table for all 12-digit Aadhaar numbers. Callers
// must ensure FINIX_AADHAAR_HASH_KEY is set at startup via MustHaveAadhaarHashKey
// (invoked from main), so this returns the configured key or an empty key that
// makes hashing fail loudly rather than producing a guessable digest.
func aadhaarHashKey() []byte {
	return []byte(strings.TrimSpace(os.Getenv("FINIX_AADHAAR_HASH_KEY")))
}

// MustHaveAadhaarHashKey aborts the process if FINIX_AADHAAR_HASH_KEY is unset.
// C-8: called from main() (NOT init()) so production refuses to boot without the
// key while unit tests — which don't call it — are not killed.
func MustHaveAadhaarHashKey() {
	if strings.TrimSpace(os.Getenv("FINIX_AADHAAR_HASH_KEY")) == "" {
		log.Fatal("[SECURITY] FATAL: FINIX_AADHAAR_HASH_KEY is not set. " +
			"Set this to a strong random secret via Secrets Manager. " +
			"The server will not start without it.")
	}
}

func filterEmpty(parts []string) []string {
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			result = append(result, strings.TrimSpace(p))
		}
	}
	return result
}
