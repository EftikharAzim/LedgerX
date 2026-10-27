-- name: CreateTransactionHeader :one
INSERT INTO transactions (user_id, currency, category_id, occurred_at, note, reversal_of)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, currency, category_id, occurred_at, note, reversal_of, created_at;

-- name: GetTransactionForUser :one
SELECT id, user_id, currency, category_id, suggested_category_id, suggestion_score,
       occurred_at, note, reversal_of, created_at
FROM transactions
WHERE id = $1 AND user_id = $2;

-- AI writes only the suggestion (never category_id), and only when the row
-- is still uncategorized — a human confirmation must never be overwritten.
-- name: SetCategorySuggestion :exec
UPDATE transactions
SET suggested_category_id = sqlc.arg(suggested_category_id),
    suggestion_score = sqlc.arg(suggestion_score)
WHERE id = sqlc.arg(id) AND category_id IS NULL;

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
       t.category_id,
       t.suggested_category_id,
       t.suggestion_score,
       p.created_at
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
WHERE p.account_id = sqlc.arg(account_id)
  AND (t.occurred_at, p.id) < (sqlc.arg(cursor_occurred_at)::timestamptz, sqlc.arg(cursor_posting_id)::bigint)
ORDER BY t.occurred_at DESC, p.id DESC
LIMIT sqlc.arg(page_limit);
