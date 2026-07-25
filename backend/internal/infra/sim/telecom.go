package sim

// Telecom VNN (Verified Number Notification) real SIM provider — REAL-READY STUB.
//
// Satisfies security.SIMVerifier and is selectable by env (SIM_PROVIDER=telecom),
// but returns ErrNotImplemented rather than faking success. Complete it by wiring
// the operator / aggregator VNN API:
//
//   Create challenge   POST https://<telecom-host>/vnn/v1/challenge
//                      headers: Authorization: Bearer <TELECOM_API>
//                      body:    { "msisdn": "+9198XXXXXXXX" }
//                      -> { "challengeId": "..." }   (operator SMSes the code)
//
//   Verify challenge   POST https://<telecom-host>/vnn/v1/verify
//                      body:    { "challengeId": "...", "code": "VNN123456" }
//                      -> { "verified": true }
//
//   SIM-swap check     GET  https://<telecom-host>/vnn/v1/sim-status?msisdn=...
//                      -> { "lastSwapAt": "...", "swapped": true|false }
//
// Read the base URL + key from env so no code change is needed to go live.

import (
	"context"
	"os"

	"FINIX/backend/internal/domain/security"
)

// TelecomConfig holds the real-provider credentials (from env).
type TelecomConfig struct {
	BaseURL string
	APIKey  string
}

// TelecomConfigFromEnv reads TELECOM_BASE_URL / TELECOM_API.
func TelecomConfigFromEnv() TelecomConfig {
	base := os.Getenv("TELECOM_BASE_URL")
	if base == "" {
		base = "https://api.telecom.example" // placeholder; set TELECOM_BASE_URL to go live
	}
	return TelecomConfig{BaseURL: base, APIKey: os.Getenv("TELECOM_API")}
}

// Telecom is the real SIM provider (unwired stub).
type Telecom struct {
	cfg TelecomConfig
}

var _ security.SIMVerifier = (*Telecom)(nil)

// NewTelecom returns the real provider as the interface type.
func NewTelecom(cfg TelecomConfig) security.SIMVerifier { return &Telecom{cfg: cfg} }

func (t *Telecom) CreateChallenge(ctx context.Context, mobile string) (string, error) {
	return "", ErrNotImplemented
}

func (t *Telecom) VerifyChallenge(ctx context.Context, mobile, challengeID, code string) (bool, error) {
	return false, ErrNotImplemented
}

func (t *Telecom) DetectSwap(ctx context.Context, userID string) (bool, error) {
	return false, ErrNotImplemented
}
