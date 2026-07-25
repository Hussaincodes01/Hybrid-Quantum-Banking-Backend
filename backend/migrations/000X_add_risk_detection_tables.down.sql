-- Sprint 1: Rollback Risk Detection Tables
DROP TABLE IF EXISTS risk_validations;
DROP TABLE IF EXISTS fraud_reports;
DROP TABLE IF EXISTS health_score_snapshots;
DROP TABLE IF EXISTS realtime_detections;

ALTER TABLE transactions DROP COLUMN IF EXISTS model_version;