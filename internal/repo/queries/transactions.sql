-- name: CreateTransactionHeader :one
INSERT INTO transactions (user_id, currency, category_id, occurred_at, note, reversal_of)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, currency, category_id, occurred_at, note, reversal_of, created_at;

-- name: GetTransactionForUser :one
SELECT id, user_id, currency, category_id, occurred_at, note, reversal_of, created_at
FROM transactions
WHERE id = $1 AND user_id = $2;

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
       p.created_at
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
WHERE p.account_id = sqlc.arg(account_id)
  AND (t.occurred_at, p.id) < (sqlc.arg(cursor_occurred_at)::timestamptz, sqlc.arg(cursor_posting_id)::bigint)
ORDER BY t.occurred_at DESC, p.id DESC
LIMIT sqlc.arg(page_limit);
