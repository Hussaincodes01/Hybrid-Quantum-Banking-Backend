CREATE TABLE IF NOT EXISTS sim_bindings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  device_id TEXT NOT NULL,
  sim_hash TEXT NOT NULL,
  ubt_token TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'active',
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sim_bindings_user ON sim_bindings(user_id);
