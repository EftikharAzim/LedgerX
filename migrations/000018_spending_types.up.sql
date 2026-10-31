ALTER TABLE transactions
  ADD COLUMN entry_type TEXT NOT NULL DEFAULT 'standard'
  CHECK (entry_type IN ('standard', 'opening_balance', 'refund'));

-- Account creation and its starting postings commit together. Keep the request
-- identity so a lost HTTP response cannot create a second account on retry.
ALTER TABLE accounts
  ADD COLUMN creation_key TEXT,
  ADD COLUMN creation_hash TEXT,
  ADD CONSTRAINT accounts_creation_identity CHECK (
    (creation_key IS NULL) = (creation_hash IS NULL)
  );
CREATE UNIQUE INDEX accounts_creation_key ON accounts(user_id, creation_key)
  WHERE creation_key IS NOT NULL;

CREATE INDEX transactions_user_occurred_id ON transactions(user_id, occurred_at DESC, id DESC);
