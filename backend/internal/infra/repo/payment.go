package repo

import (
	"context"
	"fmt"
	"time"
)

type PaymentRow struct {
	ID                   string
	UserID               string
	BeneficiaryID        string
	BeneficiaryName      string
	BeneficiaryAccount   string
	AmountPaise          int64
	Currency             string
	Method               string
	Status               string
	RiskLevel            string
	RiskScore            float64
	StartedAt            time.Time
	CompletedAt          *time.Time
	BankReference        string
	OTPVerified          bool
	UserConsent          bool
	ReceiptVerified      bool
	ReceiptVerifiedAt    *time.Time
	ReceiptHash          string
	PaymentIntentID      string
	UPIIntentID          string
	QRCodePayload        string
	SIPID                string
	StepUpChallengeID    string
	DeviceChallengeKeyID string
	CreatedAt            time.Time
}

func (r *Repo) CreatePayment(ctx context.Context, p PaymentRow) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO payments (
			user_id, beneficiary_id, beneficiary_name, beneficiary_account,
			amount_paise, currency, method, status, risk_level, risk_score,
			started_at, completed_at, bank_reference, otp_verified, user_consent,
			receipt_verified, receipt_verified_at, receipt_hash,
			payment_intent_id, upi_intent_id, qr_code_payload, sip_id,
			step_up_challenge_id, device_challenge_key_id, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
		RETURNING id`,
		p.UserID, p.BeneficiaryID, p.BeneficiaryName, p.BeneficiaryAccount,
		p.AmountPaise, p.Currency, p.Method, p.Status, p.RiskLevel, p.RiskScore,
		p.StartedAt, p.CompletedAt, p.BankReference, p.OTPVerified, p.UserConsent,
		p.ReceiptVerified, p.ReceiptVerifiedAt, p.ReceiptHash,
		p.PaymentIntentID, p.UPIIntentID, p.QRCodePayload, p.SIPID,
		p.StepUpChallengeID, p.DeviceChallengeKeyID, p.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create payment: %w", err)
	}
	return id, nil
}

func (r *Repo) ListPaymentsByUser(ctx context.Context, userID string) ([]PaymentRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, beneficiary_id, beneficiary_name, beneficiary_account,
		        amount_paise, currency, method, status, risk_level, risk_score,
		        started_at, completed_at, bank_reference, otp_verified, user_consent,
		        receipt_verified, receipt_verified_at, receipt_hash,
		        payment_intent_id, upi_intent_id, qr_code_payload, sip_id,
		        step_up_challenge_id, device_challenge_key_id, created_at
		 FROM payments WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list payments by user: %w", err)
	}
	defer rows.Close()

	var payments []PaymentRow
	for rows.Next() {
		var p PaymentRow
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.BeneficiaryID, &p.BeneficiaryName, &p.BeneficiaryAccount,
			&p.AmountPaise, &p.Currency, &p.Method, &p.Status, &p.RiskLevel, &p.RiskScore,
			&p.StartedAt, &p.CompletedAt, &p.BankReference, &p.OTPVerified, &p.UserConsent,
			&p.ReceiptVerified, &p.ReceiptVerifiedAt, &p.ReceiptHash,
			&p.PaymentIntentID, &p.UPIIntentID, &p.QRCodePayload, &p.SIPID,
			&p.StepUpChallengeID, &p.DeviceChallengeKeyID, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan payment row: %w", err)
		}
		payments = append(payments, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payments: %w", err)
	}
	return payments, nil
}

func (r *Repo) GetPaymentByID(ctx context.Context, paymentID string) (PaymentRow, error) {
	var p PaymentRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, beneficiary_id, beneficiary_name, beneficiary_account,
		        amount_paise, currency, method, status, risk_level, risk_score,
		        started_at, completed_at, bank_reference, otp_verified, user_consent,
		        receipt_verified, receipt_verified_at, receipt_hash,
		        payment_intent_id, upi_intent_id, qr_code_payload, sip_id,
		        step_up_challenge_id, device_challenge_key_id, created_at
		 FROM payments WHERE id = $1`, paymentID,
	).Scan(
		&p.ID, &p.UserID, &p.BeneficiaryID, &p.BeneficiaryName, &p.BeneficiaryAccount,
		&p.AmountPaise, &p.Currency, &p.Method, &p.Status, &p.RiskLevel, &p.RiskScore,
		&p.StartedAt, &p.CompletedAt, &p.BankReference, &p.OTPVerified, &p.UserConsent,
		&p.ReceiptVerified, &p.ReceiptVerifiedAt, &p.ReceiptHash,
		&p.PaymentIntentID, &p.UPIIntentID, &p.QRCodePayload, &p.SIPID,
		&p.StepUpChallengeID, &p.DeviceChallengeKeyID, &p.CreatedAt,
	)
	if err != nil {
		return PaymentRow{}, fmt.Errorf("get payment by id: %w", err)
	}
	return p, nil
}

func (r *Repo) ListAllPayments(ctx context.Context) ([]PaymentRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, beneficiary_id, beneficiary_name, beneficiary_account,
		        amount_paise, currency, method, status, risk_level, risk_score,
		        started_at, completed_at, bank_reference, otp_verified, user_consent,
		        receipt_verified, receipt_verified_at, receipt_hash,
		        payment_intent_id, upi_intent_id, qr_code_payload, sip_id,
		        step_up_challenge_id, device_challenge_key_id, created_at
		 FROM payments ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all payments: %w", err)
	}
	defer rows.Close()

	var payments []PaymentRow
	for rows.Next() {
		var p PaymentRow
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.BeneficiaryID, &p.BeneficiaryName, &p.BeneficiaryAccount,
			&p.AmountPaise, &p.Currency, &p.Method, &p.Status, &p.RiskLevel, &p.RiskScore,
			&p.StartedAt, &p.CompletedAt, &p.BankReference, &p.OTPVerified, &p.UserConsent,
			&p.ReceiptVerified, &p.ReceiptVerifiedAt, &p.ReceiptHash,
			&p.PaymentIntentID, &p.UPIIntentID, &p.QRCodePayload, &p.SIPID,
			&p.StepUpChallengeID, &p.DeviceChallengeKeyID, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		payments = append(payments, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payments: %w", err)
	}
	return payments, nil
}
