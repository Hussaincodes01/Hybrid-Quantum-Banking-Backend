package security

import "context"

// SIMVerifier is a swappable SIM-binding / VNN provider. The default is a local
// mock (internal/infra/sim); a real telecom VNN API can be selected by env
// later without changing callers.
//
// Provider-agnostic: no telecom-specific fields. The existing
// SIMBindingManager remains the on-device binding state machine; a SIMVerifier
// models the *external* challenge/verify/swap-detection round-trip.
type SIMVerifier interface {
	// CreateChallenge asks the provider to send a VNN challenge to the mobile
	// and returns the challenge id to correlate the response.
	CreateChallenge(ctx context.Context, mobile string) (challengeID string, err error)

	// VerifyChallenge confirms the code the user received (the telecom callback).
	// ok=false (nil err) means the code was wrong/expired.
	VerifyChallenge(ctx context.Context, mobile, challengeID, code string) (ok bool, err error)

	// DetectSwap reports whether the user's SIM has been swapped since binding.
	DetectSwap(ctx context.Context, userID string) (swapped bool, err error)
}
