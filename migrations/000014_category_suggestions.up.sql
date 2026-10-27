-- AI-assisted categorization, kept strictly outside the ledger's correctness
-- boundary: the model only ever writes a *suggestion*, never the confirmed
-- category. A human (or a deterministic rule) promotes suggested_category_id
-- into category_id. The double-entry invariants are untouched by any of this.

ALTER TABLE transactions
  ADD COLUMN suggested_category_id BIGINT REFERENCES categories(id),
  ADD COLUMN suggestion_score DOUBLE PRECISION;  -- model confidence, 0..1

-- One category name per user (case-insensitive) so the classifier's
-- candidate set is clean and confirmations are unambiguous.
CREATE UNIQUE INDEX categories_user_lower_name ON categories(user_id, lower(name));

-- Fast lookup of the work queue: transactions still awaiting a category.
CREATE INDEX idx_tx_user_uncategorized ON transactions(user_id) WHERE category_id IS NULL;
