CREATE TABLE IF NOT EXISTS url_scan_results (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES users(id),
  url TEXT NOT NULL,
  verdict TEXT NOT NULL,
  source TEXT,
  details JSONB,
  scanned_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
