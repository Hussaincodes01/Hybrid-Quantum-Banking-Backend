CREATE TABLE IF NOT EXISTS auth_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  token TEXT UNIQUE NOT NULL,
  session_start TIMESTAMPTZ,
  session_expiry TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_user ON auth_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_token ON auth_sessions(token);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expiry ON auth_sessions(session_expiry);

CREATE TABLE IF NOT EXISTS auth_challenges (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  challenge TEXT NOT NULL,
  nonce_id TEXT,
  status TEXT NOT NULL DEFAULT 'created',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  verified_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_auth_challenges_user ON auth_challenges(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_challenges_status ON auth_challenges(user_id, status);

CREATE TABLE IF NOT EXISTS freeze_states (
  user_id UUID PRIMARY KEY REFERENCES users(id),
  frozen BOOLEAN NOT NULL DEFAULT false,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS notification_settings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  category TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  UNIQUE(user_id, category)
);

CREATE INDEX IF NOT EXISTS idx_notification_settings_user ON notification_settings(user_id);

CREATE TABLE IF NOT EXISTS consent_grants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  consent_type TEXT NOT NULL,
  granted BOOLEAN NOT NULL DEFAULT false,
  UNIQUE(user_id, consent_type)
);

CREATE INDEX IF NOT EXISTS idx_consent_grants_user ON consent_grants(user_id);

CREATE TABLE IF NOT EXISTS beneficiary_links (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  recipient_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(user_id, recipient_hash)
);

CREATE INDEX IF NOT EXISTS idx_beneficiary_links_user ON beneficiary_links(user_id);

CREATE TABLE IF NOT EXISTS bank_api_connections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  connection_id TEXT UNIQUE,
  provider TEXT,
  base_url TEXT,
  client_id TEXT,
  connected BOOLEAN NOT NULL DEFAULT false,
  sandbox BOOLEAN NOT NULL DEFAULT false,
  connected_at TIMESTAMPTZ,
  last_heartbeat_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_bank_api_connections_user ON bank_api_connections(user_id);
CREATE INDEX IF NOT EXISTS idx_bank_api_connections_connection ON bank_api_connections(connection_id);

CREATE TABLE IF NOT EXISTS realtime_detections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  event_id TEXT,
  bank_account_id TEXT,
  risk_level TEXT,
  risk_score NUMERIC,
  action TEXT,
  requires_step_up BOOLEAN,
  reason TEXT,
  is_synthetic BOOLEAN,
  model_version TEXT,
  processed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_realtime_detections_user ON realtime_detections(user_id);
CREATE INDEX IF NOT EXISTS idx_realtime_detections_event ON realtime_detections(event_id);
CREATE INDEX IF NOT EXISTS idx_realtime_detections_risk ON realtime_detections(user_id, risk_level);
