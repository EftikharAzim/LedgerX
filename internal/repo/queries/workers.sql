-- name: ListAllAccountIDs :many
SELECT id FROM accounts WHERE is_active = TRUE ORDER BY id;

-- Snapshot cutoff uses created_at to match SumPostingsSince semantics.
-- name: SumPostingsUpTo :one
SELECT COALESCE(SUM(amount_minor), 0)::bigint AS total
FROM postings
WHERE account_id = sqlc.arg(account_id)
  AND created_at <= sqlc.arg(cutoff);
