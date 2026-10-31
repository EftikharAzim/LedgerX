-- name: CreateExport :one
INSERT INTO exports (user_id, month, status, report_type, start_date, end_date)
VALUES ($1, $2, 'pending', $3, $4, $5)
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
  AND t.occurred_at >= date_trunc('month', $2::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
  AND t.occurred_at < (date_trunc('month', $2::timestamptz AT TIME ZONE 'UTC') + interval '1 month') AT TIME ZONE 'UTC'
ORDER BY t.occurred_at, p.id;

-- name: ListPostingsForExport :many
SELECT t.id AS transaction_id, p.account_id, a.name AS account_name,
       t.category_id, p.amount_minor, t.currency, t.occurred_at, t.note
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
JOIN accounts a ON a.id = p.account_id
WHERE t.user_id = sqlc.arg(user_id)
  AND a.kind = 'normal'
  AND (sqlc.narg(start_date)::date IS NULL
       OR t.occurred_at >= (sqlc.narg(start_date)::date::timestamp AT TIME ZONE 'UTC'))
  AND (sqlc.narg(end_date)::date IS NULL
       OR t.occurred_at < ((sqlc.narg(end_date)::date + 1)::timestamp AT TIME ZONE 'UTC'))
ORDER BY t.occurred_at, p.id;
