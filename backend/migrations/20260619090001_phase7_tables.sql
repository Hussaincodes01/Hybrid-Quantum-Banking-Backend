-- Phase 7: Persistence for Phase 5+6 data + migration version tracking

CREATE TABLE IF NOT EXISTS schema_migrations (
    version     BIGINT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS net_worth_assets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type            TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    value_paise     BIGINT NOT NULL DEFAULT 0,
    valuation_date  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valuation_source TEXT NOT NULL DEFAULT 'user_entered',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nw_assets_user ON net_worth_assets(user_id);

CREATE TABLE IF NOT EXISTS net_worth_liabilities (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type             TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    loan_amount_paise  BIGINT NOT NULL DEFAULT 0,
    outstanding_paise  BIGINT NOT NULL DEFAULT 0,
    emi_paise        BIGINT NOT NULL DEFAULT 0,
    interest_rate    DOUBLE PRECISION NOT NULL DEFAULT 0,
    maturity_date    TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nw_liabilities_user ON net_worth_liabilities(user_id);

CREATE TABLE IF NOT EXISTS emergency_contacts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL DEFAULT '',
    relationship TEXT NOT NULL DEFAULT '',
    phone        TEXT NOT NULL DEFAULT '',
    email        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_emergency_contacts_user ON emergency_contacts(user_id);

CREATE TABLE IF NOT EXISTS feature_flags (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    flag_key  TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (user_id, flag_key)
);
CREATE INDEX IF NOT EXISTS idx_feature_flags_user ON feature_flags(user_id);

CREATE TABLE IF NOT EXISTS notification_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category    TEXT NOT NULL DEFAULT '',
    title       TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    is_read     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notification_items_user ON notification_items(user_id, created_at DESC);
