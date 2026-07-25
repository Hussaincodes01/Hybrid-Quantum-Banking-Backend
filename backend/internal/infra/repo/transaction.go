package repo

import (
	"context"
	"fmt"
	"time"
)

// TransactionRow represents a transaction record
type TransactionRow struct {
	ID               string
	UserID           string
	AmountPaise      int64
	Currency         string
	DebitCredit      string
	Recipient        string
	MerchantName     string
	MerchantCategory string
	Channel          string
	Description      string
	Status           string
	AssignedCategory string
	UserNotes        string
	RiskLevel        string
	RiskScore        float64
	XAIReason        string
	LinkedAccount    string
	PaymentIntentID  string
	UPIIntentID      string
	BankReference    string
	IdempotencyKey   string
	CoolingOffUntil  time.Time
	CreatedAt        time.Time
	ModelVersion     string
}

func (r *Repo) CreateTransaction(ctx context.Context, t TransactionRow) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO transactions (
			user_id, amount_paise, currency, debit_credit, recipient,
			merchant_name, merchant_category, channel, description, status,
			assigned_category, user_notes, risk_level, risk_score, xai_reason,
			linked_account, payment_intent_id, upi_intent_id, bank_reference,
			idempotency_key, cooling_off_until, created_at, model_version)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		 RETURNING id`,
		t.UserID, t.AmountPaise, t.Currency, t.DebitCredit, t.Recipient,
		t.MerchantName, t.MerchantCategory, t.Channel, t.Description, t.Status,
		t.AssignedCategory, t.UserNotes, t.RiskLevel, t.RiskScore, t.XAIReason,
		t.LinkedAccount, t.PaymentIntentID, t.UPIIntentID, t.BankReference,
		t.IdempotencyKey, t.CoolingOffUntil, t.CreatedAt, t.ModelVersion,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create transaction: %w", err)
	}
	return id, nil
}

func (r *Repo) ListTransactionsByUser(ctx context.Context, userID string) ([]TransactionRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, amount_paise, currency, debit_credit, recipient,
		        merchant_name, merchant_category, channel, description, status,
		        assigned_category, user_notes, risk_level, risk_score, xai_reason,
		        linked_account, payment_intent_id, upi_intent_id, bank_reference,
		        idempotency_key, cooling_off_until, created_at, model_version
		 FROM transactions WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list transactions by user: %w", err)
	}
	defer rows.Close()

	var txs []TransactionRow
	for rows.Next() {
		var t TransactionRow
		if err := rows.Scan(&t.ID, &t.UserID, &t.AmountPaise, &t.Currency, &t.DebitCredit,
			&t.Recipient, &t.MerchantName, &t.MerchantCategory, &t.Channel, &t.Description,
			&t.Status, &t.AssignedCategory, &t.UserNotes, &t.RiskLevel, &t.RiskScore,
			&t.XAIReason, &t.LinkedAccount, &t.PaymentIntentID, &t.UPIIntentID,
			&t.BankReference, &t.IdempotencyKey, &t.CoolingOffUntil, &t.CreatedAt, &t.ModelVersion); err != nil {
			return nil, fmt.Errorf("scan transaction row: %w", err)
		}
		txs = append(txs, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}
	return txs, nil
}

func (r *Repo) GetTransactionByID(ctx context.Context, txID string) (TransactionRow, error) {
	var t TransactionRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, amount_paise, currency, debit_credit, recipient,
		        merchant_name, merchant_category, channel, description, status,
		        assigned_category, user_notes, risk_level, risk_score, xai_reason,
		        linked_account, payment_intent_id, upi_intent_id, bank_reference,
		        idempotency_key, cooling_off_until, created_at, model_version
		 FROM transactions WHERE id = $1`, txID,
	).Scan(&t.ID, &t.UserID, &t.AmountPaise, &t.Currency, &t.DebitCredit,
		&t.Recipient, &t.MerchantName, &t.MerchantCategory, &t.Channel, &t.Description,
		&t.Status, &t.AssignedCategory, &t.UserNotes, &t.RiskLevel, &t.RiskScore,
		&t.XAIReason, &t.LinkedAccount, &t.PaymentIntentID, &t.UPIIntentID,
		&t.BankReference, &t.IdempotencyKey, &t.CoolingOffUntil, &t.CreatedAt, &t.ModelVersion)
	if err != nil {
		return TransactionRow{}, fmt.Errorf("get transaction by id: %w", err)
	}
	return t, nil
}

func (r *Repo) GetTransactionByIdempotencyKey(ctx context.Context, key string) (TransactionRow, bool, error) {
	var t TransactionRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, amount_paise, currency, debit_credit, recipient,
		        merchant_name, merchant_category, channel, description, status,
		        assigned_category, user_notes, risk_level, risk_score, xai_reason,
		        linked_account, payment_intent_id, upi_intent_id, bank_reference,
		        idempotency_key, cooling_off_until, created_at, model_version
		 FROM transactions WHERE idempotency_key = $1`, key,
	).Scan(&t.ID, &t.UserID, &t.AmountPaise, &t.Currency, &t.DebitCredit,
		&t.Recipient, &t.MerchantName, &t.MerchantCategory, &t.Channel, &t.Description,
		&t.Status, &t.AssignedCategory, &t.UserNotes, &t.RiskLevel, &t.RiskScore,
		&t.XAIReason, &t.LinkedAccount, &t.PaymentIntentID, &t.UPIIntentID,
		&t.BankReference, &t.IdempotencyKey, &t.CoolingOffUntil, &t.CreatedAt, &t.ModelVersion)
	if err != nil {
		return TransactionRow{}, false, fmt.Errorf("get transaction by idempotency key: %w", err)
	}
	return t, true, nil
}

func (r *Repo) ListAllTransactions(ctx context.Context) ([]TransactionRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, amount_paise, currency, debit_credit, recipient,
		        merchant_name, merchant_category, channel, description, status,
		        assigned_category, user_notes, risk_level, risk_score, xai_reason,
		        linked_account, payment_intent_id, upi_intent_id, bank_reference,
		        idempotency_key, cooling_off_until, created_at, model_version
		 FROM transactions ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all transactions: %w", err)
	}
	defer rows.Close()

	var txs []TransactionRow
	for rows.Next() {
		var t TransactionRow
		if err := rows.Scan(&t.ID, &t.UserID, &t.AmountPaise, &t.Currency, &t.DebitCredit,
			&t.Recipient, &t.MerchantName, &t.MerchantCategory, &t.Channel, &t.Description,
			&t.Status, &t.AssignedCategory, &t.UserNotes, &t.RiskLevel, &t.RiskScore,
			&t.XAIReason, &t.LinkedAccount, &t.PaymentIntentID, &t.UPIIntentID,
			&t.BankReference, &t.IdempotencyKey, &t.CoolingOffUntil, &t.CreatedAt, &t.ModelVersion); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		txs = append(txs, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}
	return txs, nil
}
