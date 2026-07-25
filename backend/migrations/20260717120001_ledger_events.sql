-- ============================================================
-- FINIX — off-chain mirror of the Hyperledger Fabric event ledger.
--
-- Fabric is the source of truth. This table is a derived read model kept in
-- sync by the backend's chaincode event listener, so the HTTP API can serve
-- dashboards / audit logs / cooling-off checks with normal SQL instead of a
-- chaincode round-trip.
--
-- tx_id + seq come from Fabric; (user_id, seq) is unique per user because the
-- chaincode assigns a monotonic per-user sequence. Re-delivered events (the
-- listener may replay from a block) upsert rather than duplicate.
-- ============================================================

CREATE TABLE IF NOT EXISTS ledger_events (
    id            BIGSERIAL PRIMARY KEY,
    event_id      TEXT        NOT NULL,
    user_id       TEXT        NOT NULL,
    category      TEXT        NOT NULL,
    action        TEXT        NOT NULL,
    resource      TEXT        NOT NULL,
    payload       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    payload_hash  TEXT        NOT NULL,
    prev_hash     TEXT        NOT NULL DEFAULT '',
    hash          TEXT        NOT NULL,
    seq           BIGINT      NOT NULL,
    tx_id         TEXT        NOT NULL,
    block_num     BIGINT      NOT NULL DEFAULT 0,
    rule_name     TEXT        NOT NULL DEFAULT '',
    rule_passed   BOOLEAN     NOT NULL DEFAULT TRUE,
    rule_violation TEXT       NOT NULL DEFAULT '',
    event_time    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT ledger_events_user_seq_key UNIQUE (user_id, seq)
);

-- Audit log / dashboard: a user's events newest-first.
CREATE INDEX IF NOT EXISTS ledger_events_user_time_idx
    ON ledger_events (user_id, event_time DESC);

-- EventsByCategory(user, category).
CREATE INDEX IF NOT EXISTS ledger_events_user_category_idx
    ON ledger_events (user_id, category, event_time DESC);

-- Cooling-off probe: latest transaction_blocked per user.
CREATE INDEX IF NOT EXISTS ledger_events_action_idx
    ON ledger_events (user_id, action, event_time DESC);

-- Fabric tx lookup (proof / reconciliation).
CREATE UNIQUE INDEX IF NOT EXISTS ledger_events_tx_seq_idx
    ON ledger_events (tx_id, seq);

-- Tracks how far the event listener has mirrored, so a restart resumes from the
-- next block instead of replaying the whole chain.
CREATE TABLE IF NOT EXISTS ledger_sync_state (
    id              INT PRIMARY KEY DEFAULT 1,
    last_block_num  BIGINT      NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ledger_sync_state_singleton CHECK (id = 1)
);

INSERT INTO ledger_sync_state (id, last_block_num)
VALUES (1, 0)
ON CONFLICT (id) DO NOTHING;
