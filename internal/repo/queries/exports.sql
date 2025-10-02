-- name: CreateExport :one
INSERT INTO exports (user_id, month, status)
VALUES ($1, $2, 'pending')
RETURNING *;

-- name: UpdateExportStatus :exec
UPDATE exports
SET status = $2, file_path = $3, updated_at = now()
WHERE id = $1;

-- name: GetExportByID :one
SELECT * FROM exports WHERE id = $1;

-- name: ListTransactionsForMonth :many
SELECT id, account_id, category_id, amount_minor, currency, occurred_at, note
FROM transactions
WHERE user_id = $1
  AND date_trunc('month', occurred_at) = date_trunc('month', $2::timestamptz)
ORDER BY occurred_at;
