-- Sprint 1: Risk Detection & Validation Tables
-- Migration: 20260725090001_risk_detection_tables.up.sql

-- realtime_detections: stores ML/heuristic fraud detections
CREATE TABLE IF NOT EXISTS realtime_detections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tx_id           UUID NOT NULL,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    detector        TEXT NOT NULL DEFAULT '',  -- 'onnx', 'heuristic', 'bert', 'gnn', 'rule'
    score           DOUBLE PRECISION NOT NULL DEFAULT 0,
    label           TEXT NOT NULL DEFAULT '',  -- 'fraud', 'mule', 'suspicious', 'safe'
    features_jsonb  JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_realtime_detections_tx ON realtime_detections(tx_id);
CREATE INDEX IF NOT EXISTS idx_realtime_detections_user ON realtime_detections(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_realtime_detections_label ON realtime_detections(label);

-- health_score_snapshots: periodic user health score snapshots
CREATE TABLE IF NOT EXISTS health_score_snapshots (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    score_300_900   INT NOT NULL DEFAULT 0,
    band            TEXT NOT NULL DEFAULT '',  -- 'poor', 'fair', 'good', 'excellent'
    pillars_jsonb   JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_health_score_snapshots_user ON health_score_snapshots(user_id, created_at DESC);

-- fraud_reports: user-submitted fraud reports
CREATE TABLE IF NOT EXISTS fraud_reports (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tx_id           UUID,  -- optional, may not have tx yet
    reporter        TEXT NOT NULL DEFAULT '',  -- 'user', 'bank', 'system'
    details         TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'open',  -- 'open', 'investigating', 'resolved', 'dismissed'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_fraud_reports_user ON fraud_reports(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fraud_reports_tx ON fraud_reports(tx_id);

-- risk_validations: validation callbacks from Python RAG service
CREATE TABLE IF NOT EXISTS risk_validations (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tx_id               UUID NOT NULL,
    agrees_with_ml      BOOLEAN NOT NULL,
    suggested_level     TEXT NOT NULL DEFAULT '',  -- 'low', 'medium', 'high', 'critical'
    confidence          DOUBLE PRECISION NOT NULL DEFAULT 0,
    rationale           TEXT NOT NULL DEFAULT '',
    recommended_action  TEXT NOT NULL DEFAULT '',  -- 'approve', 'step_up', 'cooling_off', 'block', 'dismiss'
    citations_jsonb     JSONB NOT NULL DEFAULT '[]',
    xai_explanation     TEXT NOT NULL DEFAULT '',
    applied             BOOLEAN NOT NULL DEFAULT FALSE,
    new_status          TEXT NOT NULL DEFAULT '',
    validated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_risk_validations_tx ON risk_validations(tx_id);

-- Add model_version column to transactions if not exists
-- (Safe to run multiple times)
ALTER TABLE transactions 
    ADD COLUMN IF NOT EXISTS model_version VARCHAR(64) NOT NULL DEFAULT '';