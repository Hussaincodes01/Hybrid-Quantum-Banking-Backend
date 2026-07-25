-- ============================================================
-- FINIX — JWT revocation set (spec §3.6).
-- In-memory revocation is primary; this table persists revoked token ids so a
-- logout / forced-revoke survives a backend restart. Rows are safe to purge
-- once expires_at has passed (the token would be rejected on expiry anyway).
-- ============================================================

CREATE TABLE IF NOT EXISTS jwt_revocations (
    jti         TEXT        PRIMARY KEY,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS jwt_revocations_expires_idx
    ON jwt_revocations (expires_at);
