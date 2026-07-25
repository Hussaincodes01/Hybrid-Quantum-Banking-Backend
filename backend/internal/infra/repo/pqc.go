package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PQCSessionRow struct {
	SessionID        string
	SharedSecretHash string
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

type PQCKeyRow struct {
	KeyID         string
	Algorithm     string
	PublicKeyRef  string
	KeyCreatedAt  time.Time
	KeyRotatedAt  time.Time
	RotationActor string
	IsActive      bool
}

func (r *Repo) SavePQCSession(ctx context.Context, sessionID string, sharedSecret []byte, expiresAt time.Time) error {
	secretHash := sha256HexBytes(sharedSecret)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO pqc_sessions (session_id, shared_secret_hash, expires_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (session_id) DO UPDATE SET shared_secret_hash = $2, expires_at = $3`,
		sessionID, secretHash, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("save pqc session: %w", err)
	}
	return nil
}

func (r *Repo) DeleteExpiredPQCSessions(ctx context.Context) (int64, error) {
	ct, err := r.pool.Exec(ctx,
		`DELETE FROM pqc_sessions WHERE expires_at < NOW()`,
	)
	if err != nil {
		return 0, fmt.Errorf("delete expired pqc sessions: %w", err)
	}
	return ct.RowsAffected(), nil
}

func (r *Repo) SavePQCKey(ctx context.Context, key PQCKeyRow) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO pqc_keys (key_id, algorithm, public_key_ref, key_created_at, key_rotated_at, rotation_actor, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (key_id) DO UPDATE SET key_rotated_at = $5, rotation_actor = $6, is_active = $7`,
		key.KeyID, key.Algorithm, key.PublicKeyRef, key.KeyCreatedAt, key.KeyRotatedAt, key.RotationActor, key.IsActive,
	)
	if err != nil {
		return fmt.Errorf("save pqc key: %w", err)
	}
	return nil
}

func (r *Repo) DeactivatePQCKey(ctx context.Context, keyID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE pqc_keys SET is_active = false WHERE key_id = $1`,
		keyID,
	)
	if err != nil {
		return fmt.Errorf("deactivate pqc key: %w", err)
	}
	return nil
}

func (r *Repo) GetActivePQCKey(ctx context.Context) (*PQCKeyRow, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT key_id, algorithm, public_key_ref, key_created_at, key_rotated_at, rotation_actor, is_active
		 FROM pqc_keys WHERE is_active = true ORDER BY key_rotated_at DESC LIMIT 1`,
	)
	var key PQCKeyRow
	err := row.Scan(&key.KeyID, &key.Algorithm, &key.PublicKeyRef, &key.KeyCreatedAt, &key.KeyRotatedAt, &key.RotationActor, &key.IsActive)
	if err != nil {
		return nil, fmt.Errorf("get active pqc key: %w", err)
	}
	return &key, nil
}

func sha256HexBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

var _ = pgxpool.Pool{}
