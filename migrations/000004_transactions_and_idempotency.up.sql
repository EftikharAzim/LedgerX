-- Money: store in minor units (cents). Use BIGINT, not FLOAT.
CREATE TABLE IF NOT EXISTS transactions (
  id             BIGSERIAL PRIMARY KEY,
  user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id     BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  amount_minor   BIGINT NOT NULL,  -- positive for income, negative for expense
  currency       TEXT   NOT NULL,
  category       TEXT,             -- simple for now; later use categories table FK
  occurred_at    TIMESTAMPTZ NOT NULL,
  note           TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tx_user_account_time
ON transactions(user_id, account_id, occurred_at DESC);

-- Idempotency registry: remember requests and their results per user.
CREATE TABLE IF NOT EXISTS idempotency_keys (
  user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key           TEXT   NOT NULL,
  request_hash  TEXT   NOT NULL,
  status        TEXT   NOT NULL, -- 'started' | 'succeeded' | 'failed'
  response_json TEXT,            -- cached response to return on repeat
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, key)
);
