CREATE TABLE IF NOT EXISTS loan_details (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  lender_name TEXT NOT NULL,
  loan_type TEXT NOT NULL,
  account_number TEXT,
  sanctioned_amount NUMERIC,
  outstanding_principal NUMERIC,
  interest_rate NUMERIC,
  rate_type TEXT,
  emi_amount NUMERIC,
  next_emi_date DATE,
  emis_remaining INT,
  tenure_end_date DATE,
  metadata JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
