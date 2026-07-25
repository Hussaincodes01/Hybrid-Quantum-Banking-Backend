package repo

import (
	"context"
	"fmt"
)

func (r *Repo) UpsertConsentGrant(ctx context.Context, userID, consentType string, granted bool) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO consent_grants (user_id, consent_type, granted)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, consent_type) DO UPDATE SET granted = $3`,
		userID, consentType, granted,
	)
	if err != nil {
		return fmt.Errorf("upsert consent grant: %w", err)
	}
	return nil
}

func (r *Repo) ListConsentGrantsByUser(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT consent_type, granted FROM consent_grants WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list consent grants by user: %w", err)
	}
	defer rows.Close()

	grants := make(map[string]bool)
	for rows.Next() {
		var consentType string
		var granted bool
		if err := rows.Scan(&consentType, &granted); err != nil {
			return nil, fmt.Errorf("scan consent grant row: %w", err)
		}
		grants[consentType] = granted
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate consent grants: %w", err)
	}
	return grants, nil
}
