-- name: GetMonthlySummary :one
SELECT
    COALESCE(SUM(CASE WHEN amount_minor > 0 THEN amount_minor ELSE 0 END), 0)::bigint AS inflow,
    COALESCE(SUM(CASE WHEN amount_minor < 0 THEN -amount_minor ELSE 0 END), 0)::bigint AS outflow,
    COALESCE(SUM(amount_minor), 0)::bigint AS net
FROM transactions
WHERE account_id = $1
  AND date_trunc('month', occurred_at) = date_trunc('month', $2::timestamptz);
