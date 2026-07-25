-- FINIX Backend — Complete Schema Initialization
-- Matches all 11 repo implementations in internal/infra/repo/

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- CORE TABLES
-- ============================================================

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(128) NOT NULL DEFAULT 'FINIX User',
    phone VARCHAR(16) UNIQUE NOT NULL,
    email VARCHAR(256) NOT NULL DEFAULT '',
    device_fingerprint VARCHAR(256) NOT NULL DEFAULT '',
    kin VARCHAR(64) UNIQUE,
    ubt VARCHAR(64) NOT NULL DEFAULT '',
    ekyc_verified BOOLEAN NOT NULL DEFAULT FALSE,
    biometric_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    device_bound BOOLEAN NOT NULL DEFAULT TRUE,
    sim_bound BOOLEAN NOT NULL DEFAULT TRUE,
    nudge_preference VARCHAR(32) NOT NULL DEFAULT 'moderate',
    password_hash VARCHAR(256),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sessions (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token VARCHAR(256) UNIQUE NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_holder_name VARCHAR(128) NOT NULL,
    bank_name VARCHAR(128) NOT NULL,
    branch VARCHAR(128) NOT NULL DEFAULT 'NA',
    ifsc_code VARCHAR(16) NOT NULL DEFAULT 'NA',
    masked_account_number VARCHAR(32) NOT NULL,
    upi_id VARCHAR(128) NOT NULL DEFAULT '',
    account_type VARCHAR(32) NOT NULL DEFAULT 'savings',
    verification_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    nickname VARCHAR(64) NOT NULL DEFAULT '',
    primary_account_flag BOOLEAN NOT NULL DEFAULT FALSE,
    account_token VARCHAR(128) NOT NULL DEFAULT '',
    token_provider VARCHAR(64) NOT NULL DEFAULT 'bank',
    balance_paise BIGINT NOT NULL DEFAULT 0,
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_accounts_user_id ON accounts(user_id);

CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_paise BIGINT NOT NULL,
    recipient VARCHAR(256) NOT NULL,
    channel VARCHAR(32) NOT NULL DEFAULT 'upi',
    status VARCHAR(32) NOT NULL,
    risk_level VARCHAR(16) NOT NULL,
    risk_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    xai_reason TEXT NOT NULL DEFAULT '',
    idempotency_key VARCHAR(256) UNIQUE NOT NULL,
    cooling_off_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_transactions_user_id ON transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_created ON transactions(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,
    target_amount_paise BIGINT NOT NULL,
    saved_amount_paise BIGINT NOT NULL DEFAULT 0,
    target_date TIMESTAMPTZ NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_goals_user_id ON goals(user_id);

-- ============================================================
-- SECURITY & COMPLIANCE TABLES
-- ============================================================

CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    event_type VARCHAR(128) NOT NULL,
    triggered_by VARCHAR(128) NOT NULL DEFAULT 'System',
    outcome VARCHAR(32) NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    xai_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_events_user ON audit_events(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_created ON audit_events(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS freeze_states (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    frozen BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS consent_grants (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    consent_type VARCHAR(64) NOT NULL,
    granted BOOLEAN NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, consent_type)
);
CREATE INDEX IF NOT EXISTS idx_consent_grants_user ON consent_grants(user_id);

CREATE TABLE IF NOT EXISTS challenges (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    challenge VARCHAR(256) NOT NULL,
    nonce_id VARCHAR(128) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'created',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_challenges_user ON challenges(user_id);

-- ============================================================
-- GRAPH & BENEFICIARY TABLES
-- ============================================================

CREATE TABLE IF NOT EXISTS beneficiary_links (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_hash VARCHAR(256) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, recipient_hash)
);
CREATE INDEX IF NOT EXISTS idx_beneficiary_links_user ON beneficiary_links(user_id);

-- ============================================================
-- SETTINGS & NOTIFICATIONS TABLES
-- ============================================================

CREATE TABLE IF NOT EXISTS notification_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category VARCHAR(64) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (user_id, category)
);
CREATE INDEX IF NOT EXISTS idx_notif_settings_user ON notification_settings(user_id);

-- ============================================================
-- BLOCKCHAIN LEDGER TABLE
-- ============================================================

CREATE TABLE IF NOT EXISTS blockchain_events (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(128) NOT NULL,
    resource VARCHAR(256) NOT NULL,
    payload_hash VARCHAR(256) NOT NULL,
    prev_hash VARCHAR(256) NOT NULL DEFAULT '',
    hash VARCHAR(256) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_blockchain_user ON blockchain_events(user_id);

-- ============================================================
-- AI/ML & CHATBOT TABLES
-- ============================================================

CREATE TABLE IF NOT EXISTS chatbot_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(64) NOT NULL,
    messages JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_chatbot_history_user ON chatbot_history(user_id, created_at DESC);
