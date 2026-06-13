-- Revert double-entry remodel: restore single-leg columns on transactions,
-- repopulate them from the non-external posting, then drop postings.

ALTER TABLE transactions ADD COLUMN account_id BIGINT REFERENCES accounts(id) ON DELETE CASCADE;
ALTER TABLE transactions ADD COLUMN amount_minor BIGINT;
ALTER TABLE transactions ADD COLUMN category TEXT;

UPDATE transactions t
SET account_id = p.account_id,
    amount_minor = p.amount_minor
FROM postings p
JOIN accounts a ON a.id = p.account_id AND a.kind = 'normal'
WHERE p.transaction_id = t.id;

-- Headers with no normal-account posting cannot be represented in the old
-- model; drop them rather than leave NULLs behind.
DELETE FROM transactions WHERE account_id IS NULL;

ALTER TABLE transactions ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE transactions ALTER COLUMN amount_minor SET NOT NULL;

DROP TRIGGER IF EXISTS postings_balanced ON postings;
DROP FUNCTION IF EXISTS check_transaction_balanced();
DROP TABLE postings;

DELETE FROM accounts WHERE kind = 'external';
DROP INDEX IF EXISTS accounts_one_external_per_user;
ALTER TABLE accounts DROP CONSTRAINT accounts_kind_check;
ALTER TABLE accounts DROP COLUMN kind;

TRUNCATE balance_snapshots;
