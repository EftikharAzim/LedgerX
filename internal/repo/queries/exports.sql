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
       t.category_id, p.amount_minor, t.currency, t.occurred_at, t.note,
       COALESCE(c.name, 'Uncategorized')::text AS category_name,
       (CASE WHEN t.reversal_of IS NOT NULL THEN 'reversal'
         WHEN t.entry_type <> 'standard' THEN t.entry_type
         WHEN NOT EXISTS (SELECT 1 FROM postings ep JOIN accounts ea ON ea.id = ep.account_id
           WHERE ep.transaction_id = t.id AND ea.kind = 'external') THEN 'transfer'
         WHEN p.amount_minor > 0 THEN 'income' ELSE 'expense' END)::text AS entry_type,
       t.reversal_of, reversal.id AS reversed_by, it.import_id,
       COALESCE(correction.date_only, it.date_only, false)::boolean AS date_only,
       correction.original_id AS correction_of, successor.replacement_id AS corrected_by
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
JOIN accounts a ON a.id = p.account_id
LEFT JOIN categories c ON c.id = t.category_id AND c.user_id = t.user_id
LEFT JOIN transactions reversal ON reversal.reversal_of = t.id
LEFT JOIN transaction_corrections correction ON correction.replacement_id=t.id
LEFT JOIN transaction_corrections successor ON successor.original_id=t.id
LEFT JOIN imported_transactions it ON it.transaction_id = COALESCE(correction.import_source_id,t.id)
WHERE t.user_id = sqlc.arg(user_id)
  AND a.kind = 'normal'
  AND (sqlc.narg(start_date)::date IS NULL
       OR t.occurred_at >= (sqlc.narg(start_date)::date::timestamp AT TIME ZONE 'UTC'))
  AND (sqlc.narg(end_date)::date IS NULL
       OR t.occurred_at < ((sqlc.narg(end_date)::date + 1)::timestamp AT TIME ZONE 'UTC'))
ORDER BY t.occurred_at, p.id;
