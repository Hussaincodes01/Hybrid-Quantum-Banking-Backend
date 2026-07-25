package repo

import (
	"context"
	"fmt"
)

type NotificationRow struct {
	ID       string
	UserID   string
	Category string
	Enabled  bool
}

func (r *Repo) UpsertNotificationSetting(ctx context.Context, userID, category string, enabled bool) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO notification_settings (user_id, category, enabled)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, category) DO UPDATE SET enabled = $3`,
		userID, category, enabled,
	)
	if err != nil {
		return fmt.Errorf("upsert notification setting: %w", err)
	}
	return nil
}

func (r *Repo) ListNotificationSettingsByUser(ctx context.Context, userID string) ([]NotificationRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, category, enabled FROM notification_settings WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list notification settings by user: %w", err)
	}
	defer rows.Close()

	var settings []NotificationRow
	for rows.Next() {
		var n NotificationRow
		if err := rows.Scan(&n.ID, &n.UserID, &n.Category, &n.Enabled); err != nil {
			return nil, fmt.Errorf("scan notification setting row: %w", err)
		}
		settings = append(settings, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification settings: %w", err)
	}
	return settings, nil
}
