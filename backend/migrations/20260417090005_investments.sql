CREATE TABLE IF NOT EXISTS investment_holdings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  category TEXT NOT NULL,
  instrument_id TEXT,
  name TEXT NOT NULL,
  units NUMERIC,
  avg_buy_price NUMERIC,
  current_value NUMERIC,
  absolute_return NUMERIC,
  cagr NUMERIC,
  metadata JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sip_actions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  holding_id UUID NOT NULL REFERENCES investment_holdings(id),
  action TEXT NOT NULL,
  old_amount NUMERIC,
  new_amount NUMERIC,
  otp_verified BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS investment_orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  holding_id UUID REFERENCES investment_holdings(id),
  order_type TEXT NOT NULL,
  amount NUMERIC NOT NULL,
  risk_score TEXT,
  status TEXT NOT NULL DEFAULT 'pending',
  cooling_off_until TIMESTAMPTZ,
  blockchain_tx_hash TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
