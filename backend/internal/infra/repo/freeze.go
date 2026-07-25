package repo

import (
	"context"
	"fmt"
)

func (r *Repo) SetFreezeState(ctx context.Context, userID string, frozen bool) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO freeze_states (user_id, frozen) VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE SET frozen = $2`,
		userID, frozen,
	)
	if err != nil {
		return fmt.Errorf("set freeze state: %w", err)
	}
	return nil
}

func (r *Repo) GetFreezeState(ctx context.Context, userID string) (bool, error) {
	var frozen bool
	err := r.pool.QueryRow(ctx,
		`SELECT frozen FROM freeze_states WHERE user_id = $1`, userID,
	).Scan(&frozen)
	if err != nil {
		return false, fmt.Errorf("get freeze state: %w", err)
	}
	return frozen, nil
}
