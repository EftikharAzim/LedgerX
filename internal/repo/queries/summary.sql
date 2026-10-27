-- name: GetMonthlySummary :one
SELECT
    COALESCE(SUM(CASE WHEN p.amount_minor > 0 THEN p.amount_minor ELSE 0 END), 0)::bigint AS inflow,
    COALESCE(SUM(CASE WHEN p.amount_minor < 0 THEN -p.amount_minor ELSE 0 END), 0)::bigint AS outflow,
    COALESCE(SUM(p.amount_minor), 0)::bigint AS net
FROM postings p
JOIN transactions t ON t.id = p.transaction_id
WHERE p.account_id = $1
  AND t.occurred_at >= sqlc.arg(month_start)::timestamptz
  AND t.occurred_at < sqlc.arg(month_end)::timestamptz;
