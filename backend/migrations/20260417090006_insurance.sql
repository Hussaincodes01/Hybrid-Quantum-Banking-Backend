CREATE TABLE IF NOT EXISTS insurance_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  category TEXT NOT NULL,
  insurer_name TEXT NOT NULL,
  policy_number TEXT,
  policy_type TEXT,
  sum_assured NUMERIC,
  premium_amount NUMERIC,
  premium_frequency TEXT,
  next_premium_date DATE,
  maturity_date DATE,
  nominee_name TEXT,
  nominee_contact TEXT,
  metadata JSONB,
  verified BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS insurance_claims (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  policy_id UUID NOT NULL REFERENCES insurance_policies(id),
  claim_type TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'initiated',
  amount NUMERIC,
  documents JSONB,
  timeline JSONB,
  blockchain_tx_hash TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS nominee_changes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  policy_id UUID NOT NULL REFERENCES insurance_policies(id),
  old_nominee TEXT,
  new_nominee TEXT,
  status TEXT NOT NULL DEFAULT 'pending_cooloff',
  cooloff_until TIMESTAMPTZ NOT NULL,
  otp_verified BOOLEAN DEFAULT false,
  bio_verified BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
