DROP INDEX IF EXISTS idx_tx_user_uncategorized;
DROP INDEX IF EXISTS categories_user_lower_name;
ALTER TABLE transactions DROP COLUMN suggestion_score;
ALTER TABLE transactions DROP COLUMN suggested_category_id;
