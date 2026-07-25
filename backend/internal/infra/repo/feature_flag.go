package repo

import (
	"context"
	"fmt"
)

type FeatureFlagRow struct {
	UserID  string
	FlagKey string
	Enabled bool
}

func (r *Repo) UpsertFeatureFlag(ctx context.Context, f FeatureFlagRow) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO feature_flags (user_id, flag_key, enabled)
		 VALUES ($1,$2,$3)
		 ON CONFLICT (user_id, flag_key) DO UPDATE SET enabled = $3`,
		f.UserID, f.FlagKey, f.Enabled,
	)
	if err != nil {
		return fmt.Errorf("upsert feature flag: %w", err)
	}
	return nil
}

func (r *Repo) ListFeatureFlagsByUser(ctx context.Context, userID string) ([]FeatureFlagRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, flag_key, enabled FROM feature_flags WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list feature flags: %w", err)
	}
	defer rows.Close()
	var flags []FeatureFlagRow
	for rows.Next() {
		var f FeatureFlagRow
		if err := rows.Scan(&f.UserID, &f.FlagKey, &f.Enabled); err != nil {
			return nil, fmt.Errorf("scan feature flag: %w", err)
		}
		flags = append(flags, f)
	}
	return flags, rows.Err()
}
