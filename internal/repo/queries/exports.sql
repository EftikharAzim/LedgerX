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

-- Export rows are postings on the user's normal accounts; the offsetting
-- external legs would only duplicate every line.
-- name: ListPostingsForMonth :many
SELECT t.id AS transaction_id,
       p.account_id,
       a.name AS account_name,
       t.category_id,
       p.amount_minor,
       t.currency,
       t.occurred_at,
       t.note
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
JOIN accounts a ON a.id = p.account_id
WHERE t.user_id = $1
  AND a.kind = 'normal'
  AND date_trunc('month', t.occurred_at) = date_trunc('month', $2::timestamptz)
ORDER BY t.occurred_at, p.id;
