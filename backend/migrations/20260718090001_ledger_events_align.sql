-- ============================================================
-- FINIX — align ledger_events columns with the A4 spec.
--   payload -> payload_json,  tx_id -> fabric_tx_id
-- Guarded so it is safe on both an already-created table and a fresh install.
-- ============================================================

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'ledger_events' AND column_name = 'payload'
    ) THEN
        ALTER TABLE ledger_events RENAME COLUMN payload TO payload_json;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'ledger_events' AND column_name = 'tx_id'
    ) THEN
        ALTER TABLE ledger_events RENAME COLUMN tx_id TO fabric_tx_id;
    END IF;
END $$;

-- Spec-required indexes (user_id already covered by the composite indexes).
CREATE INDEX IF NOT EXISTS ledger_events_user_idx
    ON ledger_events (user_id);
CREATE INDEX IF NOT EXISTS ledger_events_fabric_tx_idx
    ON ledger_events (fabric_tx_id);
