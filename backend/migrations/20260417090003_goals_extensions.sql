ALTER TABLE goals ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE goals ADD COLUMN IF NOT EXISTS recalculated_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS goal_milestones (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  goal_id UUID NOT NULL REFERENCES goals(id),
  milestone TEXT NOT NULL,
  reached_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS goal_history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  goal_id UUID NOT NULL REFERENCES goals(id),
  event_type TEXT NOT NULL,
  payload JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
