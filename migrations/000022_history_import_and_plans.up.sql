ALTER TABLE transactions ADD COLUMN exclude_from_reports BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE planned_transactions (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  currency TEXT NOT NULL,
  amount_minor BIGINT NOT NULL CHECK (amount_minor <> 0),
  category_id BIGINT REFERENCES categories(id) DEFERRABLE INITIALLY DEFERRED,
  occurred_at TIMESTAMPTZ NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  entry_type TEXT NOT NULL DEFAULT 'standard' CHECK (entry_type IN ('standard','refund')),
  date_only BOOLEAN NOT NULL DEFAULT FALSE,
  exclude_from_reports BOOLEAN NOT NULL DEFAULT FALSE,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','posted','canceled')),
  transaction_id BIGINT REFERENCES transactions(id) ON DELETE CASCADE,
  import_id BIGINT REFERENCES import_sessions(id) ON DELETE CASCADE,
  row_number INTEGER,
  fingerprint TEXT NOT NULL DEFAULT '',
  source_ref TEXT NOT NULL DEFAULT '',
  creation_key TEXT,
  creation_hash TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(import_id,row_number),
  UNIQUE(transaction_id),
  UNIQUE(user_id,creation_key),
  CHECK ((creation_key IS NULL)=(creation_hash IS NULL)),
  CHECK ((status='posted')=(transaction_id IS NOT NULL))
);
CREATE INDEX planned_user_due ON planned_transactions(user_id,status,occurred_at,id);
CREATE INDEX planned_import_reference ON planned_transactions(user_id,account_id,source_ref) WHERE status='pending';
