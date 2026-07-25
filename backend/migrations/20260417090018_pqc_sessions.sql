CREATE TABLE IF NOT EXISTS pqc_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id TEXT UNIQUE NOT NULL,
  shared_secret_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS pqc_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key_id TEXT UNIQUE NOT NULL,
  algorithm TEXT NOT NULL,
  public_key_ref TEXT NOT NULL,
  key_created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  key_rotated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  rotation_actor TEXT NOT NULL DEFAULT 'system',
  is_active BOOLEAN NOT NULL DEFAULT true
);

CREATE INDEX IF NOT EXISTS idx_pqc_sessions_expires ON pqc_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_pqc_keys_active ON pqc_keys(is_active);
