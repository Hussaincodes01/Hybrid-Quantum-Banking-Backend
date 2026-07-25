CREATE TABLE IF NOT EXISTS fraud_reports (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  transaction_id UUID,
  report_type TEXT NOT NULL,
  details JSONB,
  forwarded_nccp BOOLEAN DEFAULT false,
  status TEXT DEFAULT 'submitted',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
