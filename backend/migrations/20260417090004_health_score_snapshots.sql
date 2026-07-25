CREATE TABLE IF NOT EXISTS health_score_snapshots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  overall_score INT NOT NULL,
  pillars JSONB NOT NULL,
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hs_user_date ON health_score_snapshots(user_id, recorded_at DESC);
