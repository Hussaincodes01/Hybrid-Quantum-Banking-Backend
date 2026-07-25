-- Sprint 1: Risk Detection Tables + Health Score Snapshots + Fraud Reports

-- Real-time fraud detections (from rule engine + ML)
CREATE TABLE IF NOT EXISTS realtime_detections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tx_id           UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    detector        VARCHAR(64) NOT NULL,           -- 'velocity', 'gnn', 'heuristic', 'bert_sms', 'rule_engine'
    score           DOUBLE PRECISION NOT NULL,
    label           VARCHAR(32) NOT NULL,           -- 'fraud', 'suspicious', 'clean'
    features_jsonb  JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_realtime_detections_user ON realtime_detections(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_realtime_detections_tx ON realtime_detections(tx_id);

-- Health score snapshots (periodic user financial health)
CREATE TABLE IF NOT EXISTS health_score_snapshots (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    score_300_900   INT NOT NULL,                   -- 300-900 scale
    band            VARCHAR(32) NOT NULL,           -- 'poor', 'fair', 'good', 'excellent'
    pillars_jsonb   JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {spending, saving, borrowing, protection}
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_health_score_snapshots_user ON health_score_snapshots(user_id, created_at DESC);

-- Fraud reports (user-submitted)
CREATE TABLE IF NOT EXISTS fraud_reports (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tx_id           UUID REFERENCES transactions(id) ON DELETE SET NULL,
    reporter        VARCHAR(64) NOT NULL DEFAULT 'user',
    details         TEXT NOT NULL DEFAULT '',
    status          VARCHAR(32) NOT NULL DEFAULT 'open',  -- 'open', 'investigating', 'resolved', 'dismissed'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_fraud_reports_user ON fraud_reports(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fraud_reports_tx ON fraud_reports(tx_id);

-- Risk validations (from Python RAG validation callback)
CREATE TABLE IF NOT EXISTS risk_validations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tx_id           UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    agrees_with_ml  BOOLEAN NOT NULL,
    suggested_level VARCHAR(16) NOT NULL,           -- 'low', 'medium', 'high'
    confidence      DOUBLE PRECISION NOT NULL,
    rationale       TEXT NOT NULL DEFAULT '',
    recommended_action VARCHAR(64) NOT NULL,        -- 'allow', 'step_up', 'cooling_off', 'block'
    citations_jsonb JSONB NOT NULL DEFAULT '[]'::jsonb,
    xai_explanation TEXT NOT NULL DEFAULT '',
    validated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_risk_validations_tx ON risk_validations(tx_id);

-- Add model_version column to transactions
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS model_version VARCHAR(64) NOT NULL DEFAULT '';