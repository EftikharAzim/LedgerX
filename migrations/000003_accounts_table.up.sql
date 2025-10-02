CREATE TABLE IF NOT EXISTS accounts (
  id         BIGSERIAL PRIMARY KEY,
  user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT   NOT NULL,
  currency   TEXT   NOT NULL DEFAULT 'USD',
  is_active  BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT accounts_unique_name_per_user UNIQUE (user_id, name)
);

-- Handy index for listing
CREATE INDEX IF NOT EXISTS idx_accounts_user_created ON accounts(user_id, created_at DESC);