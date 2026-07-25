package repo

import (
	"context"
	"fmt"
)

func (r *Repo) CreateChallenge(ctx context.Context, userID, challenge, nonceID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO challenges (user_id, challenge, nonce_id)
		 VALUES ($1, $2, $3)`,
		userID, challenge, nonceID,
	)
	if err != nil {
		return fmt.Errorf("create challenge: %w", err)
	}
	return nil
}

func (r *Repo) GetChallengeByUserID(ctx context.Context, userID string) (challenge string, found bool, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT challenge FROM challenges WHERE user_id = $1`, userID,
	).Scan(&challenge)
	if err != nil {
		return "", false, fmt.Errorf("get challenge by user id: %w", err)
	}
	return challenge, true, nil
}

func (r *Repo) DeleteChallenge(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM challenges WHERE user_id = $1`, userID,
	)
	if err != nil {
		return fmt.Errorf("delete challenge: %w", err)
	}
	return nil
}

func (r *Repo) UpdateChallengeStatus(ctx context.Context, userID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE challenges SET status = $1 WHERE user_id = $2`, status, userID,
	)
	if err != nil {
		return fmt.Errorf("update challenge status: %w", err)
	}
	return nil
}
