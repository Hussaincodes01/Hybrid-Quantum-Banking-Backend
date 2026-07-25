CREATE TABLE IF NOT EXISTS blockchain_records (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  record_type TEXT NOT NULL,
  reference_id UUID,
  payload_hash TEXT NOT NULL,
  merkle_root TEXT,
  block_number BIGINT,
  tx_hash TEXT,
  chaincode TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bc_user_type ON blockchain_records(user_id, record_type);
