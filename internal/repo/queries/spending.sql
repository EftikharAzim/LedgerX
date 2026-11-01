-- Spending is a corrected view of recorded transactions, not gross account
-- movement. Exclude canceled originals and their reversing entries entirely.
-- Refunds reduce spending in the period received; opening money is not income.
-- name: GetSpendingByCategory :many
SELECT t.category_id, COALESCE(c.name, 'Uncategorized')::text AS category_name,
  COALESCE(SUM(CASE WHEN t.entry_type = 'standard' AND p.amount_minor > 0
    THEN p.amount_minor ELSE 0 END), 0)::bigint AS income_minor,
  COALESCE(SUM(CASE WHEN p.amount_minor < 0 THEN -p.amount_minor
    WHEN t.entry_type = 'refund' THEN -p.amount_minor ELSE 0 END), 0)::bigint AS spending_minor,
  COUNT(*) FILTER (WHERE p.amount_minor < 0 OR t.entry_type = 'refund')::bigint AS spending_count
FROM transactions t
JOIN postings p ON p.transaction_id = t.id
JOIN accounts a ON a.id = p.account_id AND a.kind = 'normal'
LEFT JOIN categories c ON c.id = t.category_id AND c.user_id = t.user_id
WHERE t.user_id = sqlc.arg(user_id)
  AND t.currency = sqlc.arg(currency)
  AND t.entry_type <> 'opening_balance'
  AND t.reversal_of IS NULL
  AND NOT EXISTS (SELECT 1 FROM transactions r WHERE r.reversal_of = t.id)
  AND EXISTS (SELECT 1 FROM postings ep JOIN accounts ea ON ea.id = ep.account_id
    WHERE ep.transaction_id = t.id AND ea.kind = 'external')
  AND (sqlc.narg(start_date)::date IS NULL OR t.occurred_at >= (sqlc.narg(start_date)::date::timestamp AT TIME ZONE 'UTC'))
  AND (sqlc.narg(end_date)::date IS NULL OR t.occurred_at < ((sqlc.narg(end_date)::date + 1)::timestamp AT TIME ZONE 'UTC'))
GROUP BY t.category_id, c.name
ORDER BY spending_minor DESC, category_name;

-- One representative normal posting per transaction. For transfers it is the
-- outgoing leg, with the other account alongside it. Account filters match
-- either leg without causing the transfer to appear twice.
-- name: ListWorkspaceActivity :many
WITH activity AS (
  SELECT p.id AS posting_id, t.id AS transaction_id, p.account_id,
    a.name AS account_name, p.amount_minor, t.currency, t.occurred_at,
    t.note, t.category_id, t.suggested_category_id, t.suggestion_score,
    t.entry_type, t.reversal_of, r.id AS reversed_by,
    it.import_id, COALESCE(it.date_only, false)::boolean AS import_date_only,
    COALESCE(other.account_id, 0)::bigint AS counterparty_account_id,
    COALESCE(other.account_name, '')::text AS counterparty_account_name,
    (other.account_id IS NOT NULL)::boolean AS is_transfer,
    CASE WHEN t.reversal_of IS NOT NULL THEN 'reversal'
      WHEN t.entry_type <> 'standard' THEN t.entry_type
      WHEN other.account_id IS NOT NULL THEN 'transfer'
      WHEN p.amount_minor > 0 THEN 'income' ELSE 'expense' END::text AS activity_type,
    COALESCE(c.name, '')::text AS category_name
  FROM transactions t
  JOIN LATERAL (
    SELECT p.id, p.account_id, p.amount_minor FROM postings p
    JOIN accounts a ON a.id = p.account_id AND a.kind = 'normal'
    WHERE p.transaction_id = t.id ORDER BY p.amount_minor, p.id LIMIT 1
  ) p ON true
  JOIN accounts a ON a.id = p.account_id
  LEFT JOIN LATERAL (
    SELECT p2.account_id, a2.name AS account_name FROM postings p2
    JOIN accounts a2 ON a2.id = p2.account_id AND a2.kind = 'normal'
    WHERE p2.transaction_id = t.id AND p2.account_id <> p.account_id
    ORDER BY p2.id LIMIT 1
  ) other ON true
  LEFT JOIN transactions r ON r.reversal_of = t.id
  LEFT JOIN imported_transactions it ON it.transaction_id = t.id
  LEFT JOIN categories c ON c.id = t.category_id AND c.user_id = t.user_id
  WHERE t.user_id = sqlc.arg(user_id)
    AND (sqlc.arg(account_id)::bigint = 0 OR EXISTS (
      SELECT 1 FROM postings fp WHERE fp.transaction_id = t.id AND fp.account_id = sqlc.arg(account_id)))
)
SELECT posting_id, transaction_id, account_id, account_name, amount_minor,
  currency, occurred_at, note, category_id, suggested_category_id,
  suggestion_score, entry_type, reversal_of, reversed_by,
  counterparty_account_id, counterparty_account_name, is_transfer, activity_type, import_id, import_date_only
FROM activity
WHERE (sqlc.arg(category_filter)::bigint = -1
    OR (sqlc.arg(category_filter)::bigint = 0 AND category_id IS NULL
      AND activity_type IN ('income', 'expense', 'refund') AND reversed_by IS NULL)
    OR category_id = sqlc.arg(category_filter))
  AND (sqlc.arg(import_filter)::bigint = 0 OR import_id = sqlc.arg(import_filter))
  AND (sqlc.arg(activity_type)::text = '' OR activity_type = sqlc.arg(activity_type))
  AND (sqlc.arg(currency)::text = '' OR currency = sqlc.arg(currency))
  AND (sqlc.arg(search)::text = '' OR position(lower(sqlc.arg(search)) IN
    lower(COALESCE(note, '') || ' ' || account_name || ' ' || COALESCE(counterparty_account_name, '') || ' ' || category_name || ' TX-' || transaction_id::text)) > 0)
  AND (sqlc.narg(start_date)::date IS NULL OR occurred_at >= (sqlc.narg(start_date)::date::timestamp AT TIME ZONE 'UTC'))
  AND (sqlc.narg(end_date)::date IS NULL OR occurred_at < ((sqlc.narg(end_date)::date + 1)::timestamp AT TIME ZONE 'UTC'))
  AND (occurred_at, transaction_id) < (sqlc.arg(cursor_occurred_at)::timestamptz, sqlc.arg(cursor_transaction_id)::bigint)
ORDER BY occurred_at DESC, transaction_id DESC
LIMIT sqlc.arg(page_limit);
