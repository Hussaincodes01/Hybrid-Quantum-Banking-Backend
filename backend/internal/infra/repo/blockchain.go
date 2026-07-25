package repo

import (
	"context"
	"fmt"
	"time"
)

type BlockchainEventRow struct {
	ID          int64
	UserID      string
	Action      string
	Resource    string
	PayloadHash string
	PrevHash    string
	Hash        string
	CreatedAt   time.Time
}

func (r *Repo) WriteBlockchainEvent(ctx context.Context, userID, action, resource, payloadHash, prevHash, hash string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO blockchain_events (user_id, action, resource, payload_hash, prev_hash, hash)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		userID, action, resource, payloadHash, prevHash, hash,
	)
	if err != nil {
		return fmt.Errorf("write blockchain event: %w", err)
	}
	return nil
}

func (r *Repo) ListBlockchainEventsByUser(ctx context.Context, userID string) ([]BlockchainEventRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, action, resource, payload_hash, prev_hash, hash, created_at
		 FROM blockchain_events WHERE user_id = $1 ORDER BY created_at ASC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list blockchain events by user: %w", err)
	}
	defer rows.Close()

	var events []BlockchainEventRow
	for rows.Next() {
		var e BlockchainEventRow
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.Resource,
			&e.PayloadHash, &e.PrevHash, &e.Hash, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan blockchain event row: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blockchain events: %w", err)
	}
	return events, nil
}

func (r *Repo) GetBlockchainMerkleRoot(ctx context.Context, userID string) (string, error) {
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT hash FROM blockchain_events
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, userID,
	).Scan(&hash)
	if err != nil {
		return "", fmt.Errorf("get blockchain merkle root: %w", err)
	}
	return hash, nil
}
