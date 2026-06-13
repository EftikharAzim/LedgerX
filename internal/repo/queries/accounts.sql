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
