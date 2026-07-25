-- Sprint 1: Risk Detection & Validation Tables (Rollback)
-- Migration: 20260725090001_risk_detection_tables.down.sql

DROP TABLE IF EXISTS risk_validations;
DROP TABLE IF EXISTS fraud_reports;
DROP TABLE IF EXISTS health_score_snapshots;
DROP TABLE IF EXISTS realtime_detections;

ALTER TABLE transactions 
    DROP COLUMN IF EXISTS model_version;