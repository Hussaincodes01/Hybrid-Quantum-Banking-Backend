package repo

import (
	"context"
	"fmt"
	"time"
)

func (r *Repo) CreateSession(ctx context.Context, userID, token string, start, expiry time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sessions (user_id, token, started_at, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		userID, token, start, expiry,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *Repo) GetSessionByToken(ctx context.Context, token string) (userID string, expiry time.Time, found bool, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT user_id, expires_at FROM sessions WHERE token = $1 AND expires_at > NOW()`, token,
	).Scan(&userID, &expiry)
	if err != nil {
		return "", time.Time{}, false, nil
	}
	return userID, expiry, true, nil
}

func (r *Repo) ListActiveSessions(ctx context.Context) (map[string]string, map[string]time.Time, map[string]time.Time, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, token, started_at, expires_at FROM sessions WHERE expires_at > NOW()`,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list active sessions: %w", err)
	}
	defer rows.Close()

	tokens := make(map[string]string)
	starts := make(map[string]time.Time)
	expiries := make(map[string]time.Time)

	for rows.Next() {
		var userID, token string
		var startedAt, expiresAt time.Time
		if err := rows.Scan(&userID, &token, &startedAt, &expiresAt); err != nil {
			return nil, nil, nil, fmt.Errorf("scan session: %w", err)
		}
		tokens[token] = userID
		starts[token] = startedAt
		expiries[token] = expiresAt
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("iterate sessions: %w", err)
	}
	return tokens, starts, expiries, nil
}

func (r *Repo) DeleteSession(ctx context.Context, token string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM sessions WHERE token = $1`, token,
	)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (r *Repo) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM sessions WHERE expires_at < NOW()`,
	)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}
