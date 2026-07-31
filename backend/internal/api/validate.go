package api

import (
	"errors"
	"regexp"
	"strings"

	"FINIX/backend/internal/domain/platform"
)

// Strict field-level input validation. decodeJSON is lenient about unknown
// fields by default (strict mode is opt-in via DECODE_STRICT_FIELDS), so this
// layer enforces formats, ranges and required-ness on the values themselves.
// Anything that does not match is rejected with a 400 before it reaches the
// domain layer.

var (
	// Indian mobile: optional +91 / 0 prefix, then 10 digits starting 6-9.
	reMobile  = regexp.MustCompile(`^(\+91|91|0)?[6-9]\d{9}$`)
	reDigits4 = regexp.MustCompile(`^\d{4}$`)
	reEmail   = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

	// Payment ceiling (₹10,00,000) — a sane per-transaction upper bound.
	maxPaymentPaise int64 = 100_000_000
)

func validateMobile(mobile string) error {
	m := strings.TrimSpace(mobile)
	if m == "" {
		return errors.New("mobile number is required")
	}
	if !reMobile.MatchString(m) {
		return errors.New("mobile number must be a valid 10-digit Indian number")
	}
	return nil
}

// validateLast4 checks a masked Aadhaar/PAN "last 4" style field.
func validateLast4(field, value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return errors.New(field + " is required")
	}
	if !reDigits4.MatchString(v) {
		return errors.New(field + " must be exactly 4 digits")
	}
	return nil
}

func validateOptionalEmail(email string) error {
	e := strings.TrimSpace(email)
	if e == "" {
		return nil // optional
	}
	if len(e) > 254 || !reEmail.MatchString(e) {
		return errors.New("email is not a valid address")
	}
	return nil
}

func validateName(name string) error {
	n := strings.TrimSpace(name)
	if len(n) > 100 {
		return errors.New("name must be 100 characters or fewer")
	}
	return nil
}

func validatePaymentAmount(paise int64) error {
	if paise <= 0 {
		return errors.New("amountPaise must be greater than zero")
	}
	if paise > maxPaymentPaise {
		return errors.New("amountPaise exceeds the per-transaction limit")
	}
	return nil
}

// ValidateTransactionRequest enforces the per-transaction invariants — a
// positive amount within the ₹10,00,000 ceiling — before the request reaches
// the domain layer, so an out-of-range amount is rejected with 400 rather than
// processed.
func ValidateTransactionRequest(req platform.InitiateTransactionRequest) error {
	return validatePaymentAmount(req.AmountPaise)
}
