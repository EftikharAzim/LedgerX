-- Store balances in minor units (BIGINT).
CREATE TABLE IF NOT EXISTS balance_snapshots (
  id          BIGSERIAL PRIMARY KEY,
  account_id  BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  as_of_date  DATE   NOT NULL,     -- UTC day boundary
  balance_minor BIGINT NOT NULL,   -- end-of-day balance
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT balance_snapshots_unique UNIQUE (account_id, as_of_date)
);

-- Index for finding most recent snapshot quickly
CREATE INDEX IF NOT EXISTS idx_balance_snapshots_recent
ON balance_snapshots(account_id, as_of_date DESC);
