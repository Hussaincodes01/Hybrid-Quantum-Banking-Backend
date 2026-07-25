CREATE TABLE IF NOT EXISTS simulation_history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  simulation_type TEXT NOT NULL,
  input_params JSONB NOT NULL,
  output_result JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
