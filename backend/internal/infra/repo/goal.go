package repo

import (
	"context"
	"fmt"
	"time"
)

type GoalRow struct {
	ID                       string
	UserID                   string
	Name                     string
	Description              string
	TargetAmountPaise        int64
	Currency                 string
	SavedAmountPaise         int64
	MonthlyContributionPaise int64
	Frequency                string
	Priority                 string
	StartDate                time.Time
	TargetDate               time.Time
	Status                   string
	LinkedAccount            string
	CreatedAt                time.Time
}

func (r *Repo) CreateGoal(ctx context.Context, g GoalRow) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO goals (user_id, name, description, target_amount_paise, currency,
		 saved_amount_paise, monthly_contribution_paise, frequency, priority,
		 start_date, target_date, status, linked_account, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 RETURNING id`,
		g.UserID, g.Name, g.Description, g.TargetAmountPaise, g.Currency,
		g.SavedAmountPaise, g.MonthlyContributionPaise, g.Frequency, g.Priority,
		g.StartDate, g.TargetDate, g.Status, g.LinkedAccount, g.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create goal: %w", err)
	}
	return id, nil
}

func (r *Repo) ListGoalsByUser(ctx context.Context, userID string) ([]GoalRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, name, description, target_amount_paise, currency,
		        saved_amount_paise, monthly_contribution_paise, frequency, priority,
		        start_date, target_date, status, linked_account, created_at
		 FROM goals WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list goals by user: %w", err)
	}
	defer rows.Close()

	var goals []GoalRow
	for rows.Next() {
		var g GoalRow
		if err := rows.Scan(&g.ID, &g.UserID, &g.Name, &g.Description, &g.TargetAmountPaise,
			&g.Currency, &g.SavedAmountPaise, &g.MonthlyContributionPaise, &g.Frequency,
			&g.Priority, &g.StartDate, &g.TargetDate, &g.Status, &g.LinkedAccount,
			&g.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan goal row: %w", err)
		}
		goals = append(goals, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate goals: %w", err)
	}
	return goals, nil
}

func (r *Repo) GetGoalByID(ctx context.Context, goalID string) (GoalRow, error) {
	var g GoalRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, name, description, target_amount_paise, currency,
		        saved_amount_paise, monthly_contribution_paise, frequency, priority,
		        start_date, target_date, status, linked_account, created_at
		 FROM goals WHERE id = $1`, goalID,
	).Scan(&g.ID, &g.UserID, &g.Name, &g.Description, &g.TargetAmountPaise,
		&g.Currency, &g.SavedAmountPaise, &g.MonthlyContributionPaise, &g.Frequency,
		&g.Priority, &g.StartDate, &g.TargetDate, &g.Status, &g.LinkedAccount,
		&g.CreatedAt)
	if err != nil {
		return GoalRow{}, fmt.Errorf("get goal by id: %w", err)
	}
	return g, nil
}

func (r *Repo) UpdateGoalSavedAmount(ctx context.Context, goalID string, savedAmount int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE goals SET saved_amount_paise = $1 WHERE id = $2`, savedAmount, goalID,
	)
	if err != nil {
		return fmt.Errorf("update goal saved amount: %w", err)
	}
	return nil
}

func (r *Repo) UpdateGoalStatus(ctx context.Context, goalID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE goals SET status = $1 WHERE id = $2`, status, goalID,
	)
	if err != nil {
		return fmt.Errorf("update goal status: %w", err)
	}
	return nil
}

func (r *Repo) ListAllGoals(ctx context.Context) ([]GoalRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, name, description, target_amount_paise, currency,
		        saved_amount_paise, monthly_contribution_paise, frequency, priority,
		        start_date, target_date, status, linked_account, created_at
		 FROM goals ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all goals: %w", err)
	}
	defer rows.Close()

	var goals []GoalRow
	for rows.Next() {
		var g GoalRow
		if err := rows.Scan(&g.ID, &g.UserID, &g.Name, &g.Description, &g.TargetAmountPaise,
			&g.Currency, &g.SavedAmountPaise, &g.MonthlyContributionPaise, &g.Frequency,
			&g.Priority, &g.StartDate, &g.TargetDate, &g.Status, &g.LinkedAccount,
			&g.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan goal: %w", err)
		}
		goals = append(goals, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate goals: %w", err)
	}
	return goals, nil
}
