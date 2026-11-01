-- name: GetSecurityState :one
SELECT id, email, password_hash, auth_version, recovery_code_hash FROM users WHERE id = $1;

-- name: GetAuthVersion :one
SELECT auth_version FROM users WHERE id = $1;

-- name: SetRecoveryCode :exec
UPDATE users SET recovery_code_hash = $2 WHERE id = $1;

-- name: ChangePassword :exec
UPDATE users SET password_hash = $2, auth_version = auth_version + 1 WHERE id = $1;

-- name: RecoverPassword :exec
UPDATE users SET password_hash = $2, recovery_code_hash = NULL, auth_version = auth_version + 1 WHERE id = $1;

-- name: LockUserExports :exec
SELECT pg_advisory_xact_lock(hashtextextended('ledger-export-user:' || sqlc.arg(user_id)::bigint::text, 0));

-- name: QueueExportDeletion :exec
INSERT INTO export_deletions(file_key)
SELECT 'export_' || id::text || '.csv' FROM exports WHERE user_id = $1
ON CONFLICT DO NOTHING;

-- name: DeleteUserOutbox :exec
DELETE FROM outbox WHERE payload->>'user_id' = sqlc.arg(user_id)::bigint::text;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1;
