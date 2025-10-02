-- name: UpsertBalanceSnapshot :exec
INSERT INTO balance_snapshots (account_id, as_of_date, balance_minor)
VALUES (sqlc.arg(account_id), sqlc.arg(as_of_date), sqlc.arg(balance_minor))
ON CONFLICT (account_id, as_of_date)
DO UPDATE SET balance_minor = EXCLUDED.balance_minor;

-- name: GetLatestSnapshot :one
SELECT id, account_id, as_of_date, balance_minor, created_at
FROM balance_snapshots
WHERE account_id = sqlc.arg(account_id)
ORDER BY as_of_date DESC
LIMIT 1;

-- name: GetSnapshotOnDate :one
SELECT id, account_id, as_of_date, balance_minor, created_at
FROM balance_snapshots
WHERE account_id = sqlc.arg(account_id) AND as_of_date = sqlc.arg(as_of_date);

-- name: SumTransactionsSince :one
SELECT COALESCE(SUM(amount_minor)::bigint, 0) AS delta
FROM transactions
WHERE account_id = sqlc.arg(account_id)
  AND occurred_at > sqlc.arg(since);
