CREATE TABLE IF NOT EXISTS investments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(256) NOT NULL,
    instrument_name VARCHAR(256) NOT NULL DEFAULT '',
    category VARCHAR(64) NOT NULL,
    quantity_units DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_purchase_price_paise BIGINT NOT NULL DEFAULT 0,
    current_value_paise BIGINT NOT NULL DEFAULT 0,
    invested_paise BIGINT NOT NULL DEFAULT 0,
    last_valuation_date TIMESTAMPTZ,
    sip_amount_paise BIGINT NOT NULL DEFAULT 0,
    sip_frequency VARCHAR(32) NOT NULL DEFAULT '',
    sip_start_date TIMESTAMPTZ,
    sip_status VARCHAR(32) NOT NULL DEFAULT 'inactive',
    broker_fund_house VARCHAR(256) NOT NULL DEFAULT '',
    dividends_paise BIGINT NOT NULL DEFAULT 0,
    tax_lot_date TIMESTAMPTZ,
    cagr DOUBLE PRECISION NOT NULL DEFAULT 0,
    recommendation_id VARCHAR(128) NOT NULL DEFAULT '',
    recommendation_explainability TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_investments_user_id ON investments(user_id);
CREATE INDEX IF NOT EXISTS idx_investments_created ON investments(user_id, created_at DESC);
