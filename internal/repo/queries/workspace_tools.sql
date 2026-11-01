-- name: GetTransactionForUserForUpdate :one
SELECT * FROM transactions WHERE id=$1 AND user_id=$2 FOR UPDATE;

-- name: GetCorrectionByKey :one
SELECT * FROM transaction_corrections WHERE user_id=$1 AND request_key=$2;

-- name: GetCorrectionOrigin :one
SELECT COALESCE(c.import_source_id, i.transaction_id, 0)::bigint AS import_source_id,
  COALESCE(c.date_only, i.date_only, false)::boolean AS date_only,
  COALESCE((SELECT r.id FROM transactions r WHERE r.reversal_of=t.id),0)::bigint AS reversed_by
FROM transactions t
LEFT JOIN transaction_corrections c ON c.replacement_id=t.id
LEFT JOIN imported_transactions i ON i.transaction_id=t.id
WHERE t.id=$1 AND t.user_id=$2;

-- name: CreateCorrection :exec
INSERT INTO transaction_corrections (user_id,original_id,reversal_id,replacement_id,import_source_id,date_only,request_key,request_hash,receipt)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: EarliestActiveAccountActivity :one
SELECT COALESCE(MIN(t.occurred_at),'9999-12-31T23:59:59Z'::timestamptz)::timestamptz AS earliest
FROM transactions t JOIN postings p ON p.transaction_id=t.id
WHERE p.account_id=$1 AND t.entry_type<>'opening_balance' AND t.reversal_of IS NULL
  AND NOT EXISTS(SELECT 1 FROM transactions r WHERE r.reversal_of=t.id);

-- name: CreateRestorePreview :one
INSERT INTO workspace_restores(user_id,filename,checksum,payload,summary) VALUES($1,$2,$3,$4,$5) RETURNING *;

-- name: GetRestoreSession :one
SELECT * FROM workspace_restores WHERE id=$1 AND user_id=$2;

-- name: GetRestoreSessionForUpdate :one
SELECT * FROM workspace_restores WHERE id=$1 AND user_id=$2 FOR UPDATE;

-- name: CompleteRestore :exec
UPDATE workspace_restores SET status='completed',payload='{}'::jsonb,result=$3,completed_at=now() WHERE id=$1 AND user_id=$2;

-- name: PruneRestorePreviews :exec
DELETE FROM workspace_restores AS old WHERE old.user_id=$1 AND old.status='preview'
 AND (old.expires_at<now() OR old.id NOT IN(SELECT recent.id FROM workspace_restores recent WHERE recent.user_id=$1 AND recent.status='preview' ORDER BY recent.id DESC LIMIT 3));

-- name: ListRestoreSessions :many
SELECT id,filename,status,summary,result,created_at,expires_at FROM workspace_restores WHERE user_id=$1 ORDER BY id DESC LIMIT 10;

-- name: WorkspaceIsEmpty :one
SELECT NOT EXISTS(SELECT 1 FROM accounts a WHERE a.user_id=sqlc.arg(user_id) AND a.kind='normal')
 AND NOT EXISTS(SELECT 1 FROM categories c WHERE c.user_id=sqlc.arg(user_id))
 AND NOT EXISTS(SELECT 1 FROM transactions t WHERE t.user_id=sqlc.arg(user_id))
 AND NOT EXISTS(SELECT 1 FROM postings p JOIN accounts a ON a.id=p.account_id WHERE a.user_id=sqlc.arg(user_id))
 AND NOT EXISTS(SELECT 1 FROM import_sessions i WHERE i.user_id=sqlc.arg(user_id) AND i.status='completed')
 AND NOT EXISTS(SELECT 1 FROM budgets budget WHERE budget.user_id=sqlc.arg(user_id))
 AND NOT EXISTS(SELECT 1 FROM balance_checks b WHERE b.user_id=sqlc.arg(user_id)) AS empty;
