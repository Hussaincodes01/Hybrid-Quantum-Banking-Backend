package repo

import (
	"context"
	"fmt"
)

func (r *Repo) ListFreezeStates(ctx context.Context) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, frozen FROM freeze_states WHERE frozen = TRUE`,
	)
	if err != nil {
		return nil, fmt.Errorf("list freeze states: %w", err)
	}
	defer rows.Close()

	states := make(map[string]bool)
	for rows.Next() {
		var userID string
		var frozen bool
		if err := rows.Scan(&userID, &frozen); err != nil {
			return nil, fmt.Errorf("scan freeze state: %w", err)
		}
		states[userID] = frozen
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate freeze states: %w", err)
	}
	return states, nil
}

func (r *Repo) ListAllConsentGrants(ctx context.Context) (map[string]map[string]bool, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, consent_type, granted FROM consent_grants`,
	)
	if err != nil {
		return nil, fmt.Errorf("list consent grants: %w", err)
	}
	defer rows.Close()

	grants := make(map[string]map[string]bool)
	for rows.Next() {
		var userID, consentType string
		var granted bool
		if err := rows.Scan(&userID, &consentType, &granted); err != nil {
			return nil, fmt.Errorf("scan consent grant: %w", err)
		}
		if grants[userID] == nil {
			grants[userID] = make(map[string]bool)
		}
		grants[userID][consentType] = granted
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate consent grants: %w", err)
	}
	return grants, nil
}

func (r *Repo) ListAllNotificationSettings(ctx context.Context) (map[string][]NotificationRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, category, enabled FROM notification_settings`,
	)
	if err != nil {
		return nil, fmt.Errorf("list notification settings: %w", err)
	}
	defer rows.Close()

	settings := make(map[string][]NotificationRow)
	for rows.Next() {
		var n NotificationRow
		if err := rows.Scan(&n.UserID, &n.Category, &n.Enabled); err != nil {
			return nil, fmt.Errorf("scan notification setting: %w", err)
		}
		settings[n.UserID] = append(settings[n.UserID], n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification settings: %w", err)
	}
	return settings, nil
}
