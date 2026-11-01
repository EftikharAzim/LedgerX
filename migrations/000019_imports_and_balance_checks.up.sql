ALTER TABLE accounts ADD COLUMN tracking_start_at TIMESTAMPTZ;
UPDATE accounts a SET tracking_start_at = (
  SELECT MIN(t.occurred_at) FROM transactions t
  JOIN postings p ON p.transaction_id = t.id
  WHERE p.account_id = a.id AND t.entry_type = 'opening_balance'
    AND t.reversal_of IS NULL
    AND NOT EXISTS (SELECT 1 FROM transactions r WHERE r.reversal_of = t.id)
);

CREATE TABLE import_sessions (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  file_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'preview' CHECK (status IN ('preview', 'completed')),
  preview JSONB NOT NULL,
  commit_hash TEXT,
  result JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '24 hours',
  completed_at TIMESTAMPTZ,
  CHECK ((status = 'completed') = (commit_hash IS NOT NULL AND result IS NOT NULL AND completed_at IS NOT NULL))
);
CREATE INDEX import_sessions_user_created ON import_sessions(user_id, created_at DESC);

CREATE TABLE imported_transactions (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  import_id BIGINT NOT NULL REFERENCES import_sessions(id) ON DELETE CASCADE,
  transaction_id BIGINT NOT NULL UNIQUE REFERENCES transactions(id) ON DELETE CASCADE,
  row_number INTEGER NOT NULL,
  fingerprint TEXT NOT NULL,
  source_ref TEXT NOT NULL DEFAULT '',
  date_only BOOLEAN NOT NULL DEFAULT FALSE,
  UNIQUE (import_id, row_number)
);
CREATE INDEX imported_transactions_fingerprint ON imported_transactions(user_id, account_id, fingerprint);
CREATE INDEX imported_transactions_reference ON imported_transactions(user_id, account_id, source_ref) WHERE source_ref <> '';

CREATE TABLE balance_checks (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  actual_minor BIGINT NOT NULL,
  recorded_minor BIGINT NOT NULL,
  difference_minor BIGINT NOT NULL,
  ledger_revision BIGINT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  request_key TEXT,
  request_hash TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((request_key IS NULL) = (request_hash IS NULL))
);
CREATE UNIQUE INDEX balance_checks_request ON balance_checks(user_id, request_key) WHERE request_key IS NOT NULL;
CREATE INDEX balance_checks_account_created ON balance_checks(account_id, created_at DESC);
