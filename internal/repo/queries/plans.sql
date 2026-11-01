-- name: GetPlanForUser :one
SELECT * FROM planned_transactions WHERE id=$1 AND user_id=$2 FOR UPDATE;

-- name: ListPlansForUser :many
SELECT p.*, a.name AS account_name, COALESCE(c.name,'Uncategorized')::text AS category_name
FROM planned_transactions p JOIN accounts a ON a.id=p.account_id
LEFT JOIN categories c ON c.id=p.category_id AND c.user_id=p.user_id
WHERE p.user_id=$1 AND p.status='pending'
ORDER BY p.occurred_at,p.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CreatePlan :one
INSERT INTO planned_transactions(user_id,account_id,currency,amount_minor,category_id,occurred_at,note,entry_type,date_only,exclude_from_reports,creation_key,creation_hash)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING *;

-- name: GetPlanByRequest :one
SELECT * FROM planned_transactions WHERE user_id=$1 AND creation_key=$2;

-- name: CancelPlan :exec
UPDATE planned_transactions SET status='canceled' WHERE id=$1 AND user_id=$2 AND status='pending';

-- name: CompletePlan :exec
UPDATE planned_transactions SET status='posted', transaction_id=$3 WHERE id=$1 AND user_id=$2 AND status='pending';

-- name: FindPlannedImportDuplicates :many
WITH requested AS MATERIALIZED (
  SELECT unnest(sqlc.arg(row_numbers)::integer[]) AS row_number,
    unnest(sqlc.arg(occurred_ats)::timestamptz[]) AS occurred_at,
    unnest(sqlc.arg(amounts)::bigint[]) AS amount_minor,
    unnest(sqlc.arg(fingerprints)::text[]) AS fingerprint,
    unnest(sqlc.arg(source_refs)::text[]) AS source_ref
), matches AS (
 SELECT requested.row_number,p.id,p.occurred_at,p.amount_minor,p.note,
   CASE WHEN requested.source_ref<>'' AND p.source_ref=requested.source_ref THEN 'reference'
     WHEN p.fingerprint=requested.fingerprint THEN 'exact' ELSE 'date_amount' END::text AS match_type,
   row_number() OVER (PARTITION BY requested.row_number ORDER BY p.id DESC) AS rank
 FROM requested JOIN planned_transactions p ON p.user_id=sqlc.arg(user_id)
   AND p.account_id=sqlc.arg(account_id) AND p.status='pending'
   AND ((requested.source_ref<>'' AND p.source_ref=requested.source_ref)
     OR p.fingerprint=requested.fingerprint
     OR ((requested.source_ref='' OR p.source_ref='') AND p.amount_minor=requested.amount_minor
       AND (p.occurred_at AT TIME ZONE 'UTC')::date=(requested.occurred_at AT TIME ZONE 'UTC')::date))
)
SELECT row_number::integer AS row_number,id,occurred_at,amount_minor,note,match_type
FROM matches WHERE rank<=3 ORDER BY row_number,id;
