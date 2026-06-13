-- Double-entry remodel.
-- A transaction becomes a pure header; money movements live in postings.
-- Every transaction's postings must sum to zero (enforced by a deferred
-- constraint trigger), so value can never be created or destroyed.

-- 1) Account kinds: 'normal' user accounts vs the per-user 'external' account
--    that represents the outside world (income sources, merchants, ...).
ALTER TABLE accounts ADD COLUMN kind TEXT NOT NULL DEFAULT 'normal';
ALTER TABLE accounts ADD CONSTRAINT accounts_kind_check CHECK (kind IN ('normal', 'external'));
CREATE UNIQUE INDEX accounts_one_external_per_user
  ON accounts(user_id) WHERE kind = 'external';

-- One external account per existing user.
INSERT INTO accounts (user_id, name, currency, kind)
SELECT u.id, 'External', 'USD', 'external'
FROM users u
WHERE NOT EXISTS (
  SELECT 1 FROM accounts a WHERE a.user_id = u.id AND a.kind = 'external'
);

-- 2) Postings: the ledger lines.
CREATE TABLE postings (
  id             BIGSERIAL PRIMARY KEY,
  transaction_id BIGINT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
  account_id     BIGINT NOT NULL REFERENCES accounts(id),
  amount_minor   BIGINT NOT NULL CHECK (amount_minor <> 0),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_postings_account_created ON postings(account_id, created_at);
CREATE INDEX idx_postings_tx ON postings(transaction_id);

-- 3) Zero-sum invariant, checked at COMMIT so multi-row inserts can land first.
CREATE OR REPLACE FUNCTION check_transaction_balanced() RETURNS trigger AS $$
DECLARE
  tx_id BIGINT;
  total BIGINT;
BEGIN
  IF TG_OP = 'DELETE' THEN
    tx_id := OLD.transaction_id;
  ELSE
    tx_id := NEW.transaction_id;
  END IF;
  SELECT COALESCE(SUM(amount_minor), 0) INTO total FROM postings WHERE transaction_id = tx_id;
  IF total <> 0 THEN
    RAISE EXCEPTION 'transaction % postings sum to %, must be 0', tx_id, total;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER postings_balanced
  AFTER INSERT OR UPDATE OR DELETE ON postings
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION check_transaction_balanced();

-- 4) Backfill: each legacy single-leg transaction becomes two postings —
--    the original account leg and an offsetting leg on the user's external
--    account. Preserve the original created_at so time-based queries hold.
INSERT INTO postings (transaction_id, account_id, amount_minor, created_at)
SELECT t.id, t.account_id, t.amount_minor, t.created_at
FROM transactions t
WHERE t.amount_minor <> 0;

INSERT INTO postings (transaction_id, account_id, amount_minor, created_at)
SELECT t.id, ext.id, -t.amount_minor, t.created_at
FROM transactions t
JOIN accounts ext ON ext.user_id = t.user_id AND ext.kind = 'external'
WHERE t.amount_minor <> 0;

-- 5) Transactions become pure headers.
ALTER TABLE transactions DROP COLUMN account_id;
ALTER TABLE transactions DROP COLUMN amount_minor;
ALTER TABLE transactions DROP COLUMN category; -- superseded by category_id

-- 6) Old snapshots were computed under occurred_at semantics; balances are
--    now derived from postings by created_at (immune to backdated entries).
--    Snapshots are derived data — drop them and let the nightly job rebuild.
TRUNCATE balance_snapshots;
