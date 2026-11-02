-- name: CreateAccount :one
INSERT INTO accounts (user_id, name, currency)
VALUES ($1, $2, $3)
RETURNING id, user_id, name, currency, is_active, kind, created_at;

-- name: CreateExternalAccount :one
INSERT INTO accounts (user_id, name, currency, kind)
VALUES ($1, 'External', $2, 'external')
RETURNING id, user_id, name, currency, is_active, kind, created_at;

-- name: GetExternalAccount :one
SELECT id, user_id, name, currency, is_active, kind, created_at
FROM accounts
WHERE user_id = $1 AND kind = 'external';

-- name: ListAccountsByUser :many
SELECT id, user_id, name, currency, is_active, kind, created_at
FROM accounts
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetAccount :one
SELECT id, user_id, name, currency, is_active, kind, created_at
FROM accounts
WHERE id = $1;

-- name: DeactivateAccount :exec
UPDATE accounts SET is_active = FALSE WHERE id = $1;

-- name: UpdateManagedAccount :one
UPDATE accounts SET name = $3, is_active = $4
WHERE id = $1 AND user_id = $2 AND kind = 'normal'
RETURNING id, user_id, name, currency, is_active, kind, created_at;

-- name: CountActiveSchedulesForAccount :one
SELECT count(*) FROM recurring_schedules
WHERE user_id = $1 AND account_id = $2 AND status = 'active';

-- name: CreateAccountWithIdentity :one
INSERT INTO accounts (user_id, name, currency, creation_key, creation_hash)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, creation_key) WHERE creation_key IS NOT NULL DO NOTHING
RETURNING id, user_id, name, currency, is_active, kind, created_at;

-- name: GetAccountByCreationKey :one
SELECT id, user_id, name, currency, is_active, kind, created_at, creation_hash
FROM accounts WHERE user_id = $1 AND creation_key = $2;

-- name: EnsureExternalAccount :one
INSERT INTO accounts (user_id, name, currency, kind)
VALUES ($1, 'External', $2, 'external')
ON CONFLICT (user_id) WHERE kind = 'external'
DO UPDATE SET user_id = EXCLUDED.user_id
RETURNING id;

-- Serialize overlapping retries before other unique constraints (such as name)
-- can win the speculative INSERT race ahead of the creation-key constraint.
-- name: LockAccountCreation :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text, 0));
