CREATE TABLE IF NOT EXISTS payments (
    id                    TEXT PRIMARY KEY,
    user_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    beneficiary_id        TEXT NOT NULL DEFAULT '',
    beneficiary_name      TEXT NOT NULL DEFAULT '',
    beneficiary_account   TEXT NOT NULL DEFAULT '',
    amount_paise          BIGINT NOT NULL DEFAULT 0,
    currency              TEXT NOT NULL DEFAULT 'INR',
    method                TEXT NOT NULL DEFAULT 'upi',
    status                TEXT NOT NULL DEFAULT 'initiated',
    risk_level            TEXT NOT NULL DEFAULT 'low',
    risk_score            REAL NOT NULL DEFAULT 0,
    started_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at          TIMESTAMPTZ,
    bank_reference        TEXT NOT NULL DEFAULT '',
    otp_verified          BOOLEAN NOT NULL DEFAULT FALSE,
    user_consent          BOOLEAN NOT NULL DEFAULT FALSE,
    receipt_verified      BOOLEAN NOT NULL DEFAULT FALSE,
    receipt_verified_at   TIMESTAMPTZ,
    receipt_hash          TEXT NOT NULL DEFAULT '',
    payment_intent_id     TEXT NOT NULL DEFAULT '',
    upi_intent_id         TEXT NOT NULL DEFAULT '',
    qr_code_payload       TEXT NOT NULL DEFAULT '',
    sip_id                TEXT NOT NULL DEFAULT '',
    step_up_challenge_id  TEXT NOT NULL DEFAULT '',
    device_challenge_key_id TEXT NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payments_user_id ON payments(user_id);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
