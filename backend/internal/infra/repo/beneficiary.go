package repo

import (
	"context"
	"fmt"
)

func (r *Repo) CreateBeneficiaryLink(ctx context.Context, userID, recipientHash string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO beneficiary_links (user_id, recipient_hash)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, recipient_hash) DO NOTHING`,
		userID, recipientHash,
	)
	if err != nil {
		return fmt.Errorf("create beneficiary link: %w", err)
	}
	return nil
}

func (r *Repo) IsBeneficiaryKnown(ctx context.Context, userID, recipientHash string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM beneficiary_links WHERE user_id = $1 AND recipient_hash = $2)`,
		userID, recipientHash,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check beneficiary known: %w", err)
	}
	return exists, nil
}

func (r *Repo) ListBeneficiaryLinks(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT recipient_hash FROM beneficiary_links WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list beneficiary links: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("scan beneficiary link row: %w", err)
		}
		hashes = append(hashes, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate beneficiary links: %w", err)
	}
	return hashes, nil
}
