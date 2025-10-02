-- name: CreateTransaction :one
INSERT INTO transactions (user_id, account_id, amount_minor, currency, category, occurred_at, note)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, user_id, account_id, amount_minor, currency, category, occurred_at, note, created_at;

-- name: ListTransactionsByAccount :many
SELECT id, user_id, account_id, amount_minor, currency, category, occurred_at, note, created_at
FROM transactions
WHERE user_id = $1 AND account_id = $2
ORDER BY occurred_at DESC, id DESC
LIMIT $3 OFFSET $4;
