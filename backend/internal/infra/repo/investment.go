package repo

import (
	"context"
	"fmt"
	"time"
)

type InvestmentRow struct {
	ID                           string
	UserID                       string
	Name                         string
	InstrumentName               string
	Category                     string
	QuantityUnits                float64
	AvgPurchasePricePaise        int64
	CurrentValuePaise            int64
	InvestedPaise                int64
	LastValuationDate            time.Time
	SIPAmountPaise               int64
	SIPFrequency                 string
	SIPStartDate                 time.Time
	SIPStatus                    string
	BrokerFundHouse              string
	DividendsPaise               int64
	TaxLotDate                   time.Time
	CAGR                         float64
	RecommendationID             string
	RecommendationExplainability string
	CreatedAt                    time.Time
}

func (r *Repo) CreateInvestment(ctx context.Context, inv InvestmentRow) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO investments (
			user_id, name, instrument_name, category, quantity_units,
			avg_purchase_price_paise, current_value_paise, invested_paise,
			last_valuation_date, sip_amount_paise, sip_frequency, sip_start_date,
			sip_status, broker_fund_house, dividends_paise, tax_lot_date,
			cagr, recommendation_id, recommendation_explainability, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		 RETURNING id`,
		inv.UserID, inv.Name, inv.InstrumentName, inv.Category, inv.QuantityUnits,
		inv.AvgPurchasePricePaise, inv.CurrentValuePaise, inv.InvestedPaise,
		inv.LastValuationDate, inv.SIPAmountPaise, inv.SIPFrequency, inv.SIPStartDate,
		inv.SIPStatus, inv.BrokerFundHouse, inv.DividendsPaise, inv.TaxLotDate,
		inv.CAGR, inv.RecommendationID, inv.RecommendationExplainability, inv.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create investment: %w", err)
	}
	return id, nil
}

func (r *Repo) ListInvestmentsByUser(ctx context.Context, userID string) ([]InvestmentRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, name, instrument_name, category, quantity_units,
		        avg_purchase_price_paise, current_value_paise, invested_paise,
		        last_valuation_date, sip_amount_paise, sip_frequency, sip_start_date,
		        sip_status, broker_fund_house, dividends_paise, tax_lot_date,
		        cagr, recommendation_id, recommendation_explainability, created_at
		 FROM investments WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list investments by user: %w", err)
	}
	defer rows.Close()

	var invs []InvestmentRow
	for rows.Next() {
		var inv InvestmentRow
		if err := rows.Scan(&inv.ID, &inv.UserID, &inv.Name, &inv.InstrumentName,
			&inv.Category, &inv.QuantityUnits, &inv.AvgPurchasePricePaise,
			&inv.CurrentValuePaise, &inv.InvestedPaise, &inv.LastValuationDate,
			&inv.SIPAmountPaise, &inv.SIPFrequency, &inv.SIPStartDate, &inv.SIPStatus,
			&inv.BrokerFundHouse, &inv.DividendsPaise, &inv.TaxLotDate, &inv.CAGR,
			&inv.RecommendationID, &inv.RecommendationExplainability, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan investment row: %w", err)
		}
		invs = append(invs, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investments: %w", err)
	}
	return invs, nil
}

func (r *Repo) GetInvestmentByID(ctx context.Context, invID string) (InvestmentRow, error) {
	var inv InvestmentRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, name, instrument_name, category, quantity_units,
		        avg_purchase_price_paise, current_value_paise, invested_paise,
		        last_valuation_date, sip_amount_paise, sip_frequency, sip_start_date,
		        sip_status, broker_fund_house, dividends_paise, tax_lot_date,
		        cagr, recommendation_id, recommendation_explainability, created_at
		 FROM investments WHERE id = $1`, invID,
	).Scan(&inv.ID, &inv.UserID, &inv.Name, &inv.InstrumentName,
		&inv.Category, &inv.QuantityUnits, &inv.AvgPurchasePricePaise,
		&inv.CurrentValuePaise, &inv.InvestedPaise, &inv.LastValuationDate,
		&inv.SIPAmountPaise, &inv.SIPFrequency, &inv.SIPStartDate, &inv.SIPStatus,
		&inv.BrokerFundHouse, &inv.DividendsPaise, &inv.TaxLotDate, &inv.CAGR,
		&inv.RecommendationID, &inv.RecommendationExplainability, &inv.CreatedAt)
	if err != nil {
		return InvestmentRow{}, fmt.Errorf("get investment by id: %w", err)
	}
	return inv, nil
}

func (r *Repo) ListAllInvestments(ctx context.Context) ([]InvestmentRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, name, instrument_name, category, quantity_units,
		        avg_purchase_price_paise, current_value_paise, invested_paise,
		        last_valuation_date, sip_amount_paise, sip_frequency, sip_start_date,
		        sip_status, broker_fund_house, dividends_paise, tax_lot_date,
		        cagr, recommendation_id, recommendation_explainability, created_at
		 FROM investments ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all investments: %w", err)
	}
	defer rows.Close()

	var invs []InvestmentRow
	for rows.Next() {
		var inv InvestmentRow
		if err := rows.Scan(&inv.ID, &inv.UserID, &inv.Name, &inv.InstrumentName,
			&inv.Category, &inv.QuantityUnits, &inv.AvgPurchasePricePaise,
			&inv.CurrentValuePaise, &inv.InvestedPaise, &inv.LastValuationDate,
			&inv.SIPAmountPaise, &inv.SIPFrequency, &inv.SIPStartDate, &inv.SIPStatus,
			&inv.BrokerFundHouse, &inv.DividendsPaise, &inv.TaxLotDate, &inv.CAGR,
			&inv.RecommendationID, &inv.RecommendationExplainability, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan investment: %w", err)
		}
		invs = append(invs, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investments: %w", err)
	}
	return invs, nil
}
