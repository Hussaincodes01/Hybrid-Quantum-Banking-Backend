package repo

import (
	"context"
	"fmt"
	"time"
)

type AuditEventRow struct {
	ID          int64
	UserID      string
	EventType   string
	TriggeredBy string
	Outcome     string
	Details     string
	XAIReason   string
	CreatedAt   time.Time
}

func (r *Repo) CreateAuditEvent(ctx context.Context, e AuditEventRow) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO audit_events (user_id, event_type, triggered_by, outcome, details, xai_reason, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING id`,
		e.UserID, e.EventType, e.TriggeredBy, e.Outcome, e.Details, e.XAIReason, e.CreatedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create audit event: %w", err)
	}
	return id, nil
}

func (r *Repo) ListAuditEventsByUser(ctx context.Context, userID string, limit int) ([]AuditEventRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, event_type, triggered_by, outcome, details, xai_reason, created_at
		 FROM audit_events WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT $2`, userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit events by user: %w", err)
	}
	defer rows.Close()

	var events []AuditEventRow
	for rows.Next() {
		var e AuditEventRow
		if err := rows.Scan(&e.ID, &e.UserID, &e.EventType, &e.TriggeredBy,
			&e.Outcome, &e.Details, &e.XAIReason, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit event row: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}

func (r *Repo) CountAuditEventsByUser(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_events WHERE user_id = $1`, userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count audit events by user: %w", err)
	}
	return count, nil
}
