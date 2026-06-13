-- Reversals keep the ledger append-only: a mistake is corrected by a new
-- transaction whose postings negate the original. The UNIQUE constraint
-- guarantees a transaction can be reversed at most once.
ALTER TABLE transactions
  ADD COLUMN reversal_of BIGINT UNIQUE REFERENCES transactions(id);
