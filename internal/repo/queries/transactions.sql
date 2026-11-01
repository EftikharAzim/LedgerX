-- name: CreateTransactionHeader :one
INSERT INTO transactions (user_id, currency, category_id, occurred_at, note, reversal_of, entry_type, exclude_from_reports)
VALUES ($1, $2, $3, $4, $5, $6, COALESCE(NULLIF(sqlc.arg(entry_type)::text, ''), 'standard'), sqlc.arg(exclude_from_reports)::boolean)
RETURNING id, user_id, currency, category_id, occurred_at, note, reversal_of, created_at, exclude_from_reports;

-- name: GetTransactionForUser :one
SELECT id, user_id, currency, category_id, suggested_category_id, suggestion_score,
       occurred_at, note, reversal_of, created_at, entry_type, exclude_from_reports
FROM transactions
WHERE id = $1 AND user_id = $2;

-- AI writes only the suggestion (never category_id), and only when the row
-- is still uncategorized — a human confirmation must never be overwritten.
-- name: SetCategorySuggestion :exec
UPDATE transactions
SET suggested_category_id = sqlc.arg(suggested_category_id),
    suggestion_score = sqlc.arg(suggestion_score)
WHERE transactions.id = sqlc.arg(id) AND transactions.category_id IS NULL
 AND EXISTS(SELECT 1 FROM categories c LEFT JOIN categories parent ON parent.id=c.parent_id
 WHERE c.id=sqlc.arg(suggested_category_id) AND c.user_id=transactions.user_id
 AND NOT c.deleted AND NOT c.archived AND COALESCE(parent.archived,false)=false);

-- Human confirmation: promote a category (typically the suggested one) into
-- the authoritative column and clear the suggestion. Ownership-checked.
-- name: ConfirmTransactionCategory :one
UPDATE transactions
SET category_id = sqlc.arg(category_id),
    suggested_category_id = NULL,
    suggestion_score = NULL
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id)
RETURNING id, user_id, currency, category_id, occurred_at, note, reversal_of, created_at;

-- Net movement on the user's real (non-external) accounts for a transaction:
-- the signed amount a human thinks of (income +, expense -). Used as a signal
-- for categorization.
-- name: GetTransactionNormalAmount :one
SELECT COALESCE(SUM(p.amount_minor), 0)::bigint AS amount_minor
FROM postings p
JOIN accounts a ON a.id = p.account_id
WHERE p.transaction_id = $1 AND a.kind = 'normal';

-- name: CreatePosting :one
INSERT INTO postings (transaction_id, account_id, amount_minor)
VALUES ($1, $2, $3)
RETURNING id, transaction_id, account_id, amount_minor, created_at;

-- name: ListPostingsByTransaction :many
SELECT id, transaction_id, account_id, amount_minor, created_at
FROM postings
WHERE transaction_id = $1
ORDER BY id;

-- Ledger entries for one account, newest first, keyset-paginated on
-- (occurred_at, posting id). Pass cursor values of 'infinity'/max id
-- for the first page.
-- name: ListAccountEntries :many
SELECT p.id AS posting_id,
       t.id AS transaction_id,
       p.account_id,
       p.amount_minor,
       t.currency,
       t.occurred_at,
       t.note,
       t.entry_type,
       t.exclude_from_reports,
       it.import_id,
       COALESCE(correction.date_only, it.date_only, false)::boolean AS import_date_only,
       correction.original_id AS correction_of, successor.replacement_id AS corrected_by,
       t.category_id,
       t.suggested_category_id,
       t.suggestion_score,
       t.reversal_of,
       reversal.id AS reversed_by,
       (source.kind = 'normal' AND EXISTS (
           SELECT 1 FROM postings other
           JOIN accounts destination ON destination.id = other.account_id
           WHERE other.transaction_id = t.id
             AND other.account_id <> p.account_id
             AND destination.kind = 'normal'
       )) AS is_transfer,
       p.created_at
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
JOIN accounts source ON source.id = p.account_id
LEFT JOIN transactions reversal ON reversal.reversal_of = t.id
LEFT JOIN transaction_corrections correction ON correction.replacement_id=t.id
LEFT JOIN transaction_corrections successor ON successor.original_id=t.id
LEFT JOIN imported_transactions it ON it.transaction_id = COALESCE(correction.import_source_id,t.id)
WHERE p.account_id = sqlc.arg(account_id)
  AND (t.occurred_at, p.id) < (sqlc.arg(cursor_occurred_at)::timestamptz, sqlc.arg(cursor_posting_id)::bigint)
ORDER BY t.occurred_at DESC, p.id DESC
LIMIT sqlc.arg(page_limit);
