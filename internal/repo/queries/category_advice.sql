-- Only confirmed, uncanceled personal income/expenses/refunds teach this hint.
-- Both sides use the same normalization; a note is matched exactly, not guessed
-- from merchant fragments. No other user's records or category names are read.
-- name: ListConfirmedCategoryMatches :many
SELECT COALESCE(t.category_id, 0)::bigint AS category_id
FROM transactions t
JOIN postings p ON p.transaction_id = t.id
JOIN accounts a ON a.id = p.account_id AND a.kind = 'normal' AND a.user_id = t.user_id
JOIN categories c ON c.id = t.category_id AND c.user_id = t.user_id
WHERE t.user_id = sqlc.arg(user_id)
  AND t.currency = sqlc.arg(currency)
  AND t.category_id IS NOT NULL AND NOT c.archived AND NOT c.deleted
  AND NOT EXISTS (SELECT 1 FROM categories parent WHERE parent.id=c.parent_id AND (parent.archived OR parent.deleted))
  AND t.reversal_of IS NULL
  AND NOT EXISTS (SELECT 1 FROM transactions r WHERE r.reversal_of = t.id)
  AND lower(regexp_replace(btrim(COALESCE(t.note, '')), '[[:space:]]+', ' ', 'g'))
    = lower(regexp_replace(btrim(sqlc.arg(note)::text), '[[:space:]]+', ' ', 'g'))
  AND ((sqlc.arg(kind)::text = 'expense' AND t.entry_type = 'standard' AND p.amount_minor < 0)
    OR (sqlc.arg(kind)::text = 'income' AND t.entry_type = 'standard' AND p.amount_minor > 0)
    OR (sqlc.arg(kind)::text = 'refund' AND t.entry_type = 'refund' AND p.amount_minor > 0))
  AND EXISTS (SELECT 1 FROM postings ep JOIN accounts ea ON ea.id = ep.account_id
    WHERE ep.transaction_id = t.id AND ea.kind = 'external')
ORDER BY t.created_at DESC, t.id DESC
LIMIT 20;
