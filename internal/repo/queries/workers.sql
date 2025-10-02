-- name: ListAllAccountIDs :many
SELECT id FROM accounts WHERE is_active = TRUE ORDER BY id;

-- name: SumTransactionsUpTo :one
SELECT COALESCE(SUM(amount_minor)::bigint, 0) AS total
FROM transactions
WHERE account_id = sqlc.arg(account_id)
  AND occurred_at <= sqlc.arg(cutoff);
