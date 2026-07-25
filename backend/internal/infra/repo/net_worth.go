package repo

import (
	"context"
	"fmt"
	"time"
)

type NetWorthAssetRow struct {
	ID              string
	UserID          string
	Type            string
	Description     string
	ValuePaise      int64
	ValuationDate   time.Time
	ValuationSource string
}

type NetWorthLiabilityRow struct {
	ID               string
	UserID           string
	Type             string
	Description      string
	LoanAmountPaise  int64
	OutstandingPaise int64
	EMIPaise         int64
	InterestRate     float64
	MaturityDate     *time.Time
}

func (r *Repo) CreateNetWorthAsset(ctx context.Context, a NetWorthAssetRow, userID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO net_worth_assets (user_id, type, description, value_paise, valuation_date, valuation_source)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		userID, a.Type, a.Description, a.ValuePaise, a.ValuationDate, a.ValuationSource,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create net worth asset: %w", err)
	}
	return id, nil
}

func (r *Repo) ListNetWorthAssetsByUser(ctx context.Context, userID string) ([]NetWorthAssetRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, type, description, value_paise, valuation_date, valuation_source
		 FROM net_worth_assets WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list net worth assets: %w", err)
	}
	defer rows.Close()
	var assets []NetWorthAssetRow
	for rows.Next() {
		var a NetWorthAssetRow
		if err := rows.Scan(&a.ID, &a.UserID, &a.Type, &a.Description, &a.ValuePaise, &a.ValuationDate, &a.ValuationSource); err != nil {
			return nil, fmt.Errorf("scan net worth asset: %w", err)
		}
		assets = append(assets, a)
	}
	return assets, rows.Err()
}

func (r *Repo) DeleteNetWorthAsset(ctx context.Context, assetID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM net_worth_assets WHERE id = $1`, assetID)
	if err != nil {
		return fmt.Errorf("delete net worth asset: %w", err)
	}
	return nil
}

func (r *Repo) ListAllNetWorthAssets(ctx context.Context) (map[string][]NetWorthAssetRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, user_id, type, description, value_paise, valuation_date, valuation_source FROM net_worth_assets`)
	if err != nil {
		return nil, fmt.Errorf("list all net worth assets: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]NetWorthAssetRow)
	for rows.Next() {
		var a NetWorthAssetRow
		if err := rows.Scan(&a.ID, &a.UserID, &a.Type, &a.Description, &a.ValuePaise, &a.ValuationDate, &a.ValuationSource); err != nil {
			return nil, fmt.Errorf("scan net worth asset: %w", err)
		}
		result[a.UserID] = append(result[a.UserID], a)
	}
	return result, rows.Err()
}
