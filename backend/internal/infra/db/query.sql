-- FINIX Backend SQLC Queries
-- All CRUD operations for the 16+ entity tables

-- name: CreateUser :one
INSERT INTO users (name, phone, email, kin, ubt, ekyc_verified, biometric_enabled, device_bound, sim_bound, nudge_preference)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, name, phone, email, kin, ubt, ekyc_verified, biometric_enabled, device_bound, sim_bound, nudge_preference, created_at;

-- name: GetUserByPhone :one
SELECT id, name, phone, email, kin, ubt, ekyc_verified, biometric_enabled, device_bound, sim_bound, nudge_preference, created_at
FROM users WHERE phone = $1;

-- name: GetUserByID :one
SELECT id, name, phone, email, kin, ubt, ekyc_verified, biometric_enabled, device_bound, sim_bound, nudge_preference, created_at
FROM users WHERE id = $1;

-- name: UpdateUser :exec
UPDATE users SET name = $2, email = $3, kin = $4, ekyc_verified = $5, biometric_enabled = $6, device_bound = $7, sim_bound = $8, nudge_preference = $9
WHERE id = $1;

-- name: CreateSession :exec
INSERT INTO sessions (user_id, token, started_at, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetSessionByToken :one
SELECT user_id, token, started_at, expires_at FROM sessions WHERE token = $1 AND expires_at > NOW();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= NOW();

-- name: CreateAccount :one
INSERT INTO accounts (user_id, account_holder_name, bank_name, branch, ifsc_code, masked_account_number, upi_id, account_type, verification_status, nickname, primary_account_flag, account_token, token_provider, balance_paise, linked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING id;

-- name: ListAccountsByUser :many
SELECT id, user_id, account_holder_name, bank_name, branch, ifsc_code, masked_account_number, upi_id, account_type, verification_status, nickname, primary_account_flag, account_token, token_provider, balance_paise, linked_at
FROM accounts WHERE user_id = $1 ORDER BY linked_at;

-- name: UpdateAccount :exec
UPDATE accounts SET nickname = $3, primary_account_flag = $4, verification_status = $5
WHERE id = $1 AND user_id = $2;

-- name: CreateTransaction :one
INSERT INTO transactions (user_id, amount_paise, currency, debit_credit, recipient, merchant_name, merchant_category, channel, description, status, assigned_category, user_notes, risk_level, risk_score, xai_reason, linked_account, payment_intent_id, upi_intent_id, bank_reference, idempotency_key, cooling_off_until, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
RETURNING id;

-- name: ListTransactionsByUser :many
SELECT id, user_id, amount_paise, currency, debit_credit, recipient, merchant_name, merchant_category, channel, description, status, assigned_category, user_notes, risk_level, risk_score, xai_reason, linked_account, payment_intent_id, upi_intent_id, bank_reference, idempotency_key, cooling_off_until, created_at
FROM transactions WHERE user_id = $1 ORDER BY created_at DESC;

-- name: GetTransactionByID :one
SELECT id, user_id, amount_paise, currency, debit_credit, recipient, merchant_name, merchant_category, channel, description, status, assigned_category, user_notes, risk_level, risk_score, xai_reason, linked_account, payment_intent_id, upi_intent_id, bank_reference, idempotency_key, cooling_off_until, created_at
FROM transactions WHERE id = $1 AND user_id = $2;

-- name: CreateGoal :one
INSERT INTO goals (user_id, name, description, target_amount_paise, currency, saved_amount_paise, monthly_contribution_paise, frequency, priority, start_date, target_date, status, linked_account, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id;

-- name: ListGoalsByUser :many
SELECT id, user_id, name, description, target_amount_paise, currency, saved_amount_paise, monthly_contribution_paise, frequency, priority, start_date, target_date, status, linked_account, created_at
FROM goals WHERE user_id = $1 ORDER BY created_at;

-- name: UpdateGoal :exec
UPDATE goals SET saved_amount_paise = $3, status = $4 WHERE id = $1 AND user_id = $2;

-- name: SetFreezeState :exec
INSERT INTO freeze_state (user_id, frozen) VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET frozen = $2, updated_at = NOW();

-- name: GetFreezeState :one
SELECT frozen FROM freeze_state WHERE user_id = $1;

-- name: UpsertConsentGrant :exec
INSERT INTO consents (user_id, consent_type, granted) VALUES ($1, $2, $3)
ON CONFLICT (user_id, consent_type) DO UPDATE SET granted = $3, updated_at = NOW();

-- name: GetConsentsByUser :many
SELECT consent_type, granted FROM consents WHERE user_id = $1;

-- name: CreateAuditEvent :one
INSERT INTO audit_events (user_id, event_type, triggered_by, outcome, details, xai_reason, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id;

-- name: ListAuditEventsByUser :many
SELECT id, user_id, event_type, triggered_by, outcome, details, xai_reason, created_at
FROM audit_events WHERE user_id = $1 ORDER BY created_at DESC;

-- name: WriteBlockchainEvent :exec
INSERT INTO blockchain_events (user_id, action, resource, payload_hash, prev_hash, hash)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListBlockchainEventsByUser :many
SELECT id, user_id, action, resource, payload_hash, prev_hash, hash, created_at
FROM blockchain_events WHERE user_id = $1 ORDER BY created_at;

-- name: CreateBeneficiaryLink :exec
INSERT INTO beneficiary_graph (user_id, recipient_hash) VALUES ($1, $2)
ON CONFLICT (user_id, recipient_hash) DO NOTHING;

-- name: CreateNotification :exec
INSERT INTO notifications (user_id, category, enabled) VALUES ($1, $2, $3)
ON CONFLICT (user_id, category) DO UPDATE SET enabled = $3;

-- name: GetNotificationSettingsByUser :many
SELECT category, enabled FROM notifications WHERE user_id = $1;

-- name: SaveChatHistory :exec
INSERT INTO chatbot_history (user_id, session_id, messages, created_at) VALUES ($1, $2, $3, $4);

-- name: GetChatHistoryByUser :many
SELECT session_id, messages, created_at FROM chatbot_history WHERE user_id = $1 ORDER BY created_at DESC;

-- name: CreateInvestment :one
INSERT INTO investments (user_id, name, instrument_name, category, quantity_units, avg_purchase_price_paise, current_value_paise, invested_paise, last_valuation_date, sip_amount_paise, sip_frequency, sip_start_date, sip_status, broker_fund_house, dividends_paise, tax_lot_date, cagr, recommendation_id, recommendation_explainability, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
RETURNING id;

-- name: ListInvestmentsByUser :many
SELECT id, user_id, name, instrument_name, category, quantity_units, avg_purchase_price_paise, current_value_paise, invested_paise, last_valuation_date, sip_amount_paise, sip_frequency, sip_start_date, sip_status, broker_fund_house, dividends_paise, tax_lot_date, cagr, recommendation_id, recommendation_explainability, created_at
FROM investments WHERE user_id = $1 ORDER BY created_at DESC;

-- name: GetInvestmentByID :one
SELECT id, user_id, name, instrument_name, category, quantity_units, avg_purchase_price_paise, current_value_paise, invested_paise, last_valuation_date, sip_amount_paise, sip_frequency, sip_start_date, sip_status, broker_fund_house, dividends_paise, tax_lot_date, cagr, recommendation_id, recommendation_explainability, created_at
FROM investments WHERE id = $1 AND user_id = $2;
