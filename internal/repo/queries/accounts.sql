-- name: CreateAccount :one
INSERT INTO accounts (user_id, name, currency)
VALUES ($1, $2, $3)
RETURNING id, user_id, name, currency, is_active, created_at;

-- name: ListAccountsByUser :many
SELECT id, user_id, name, currency, is_active, created_at
FROM accounts
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetAccount :one
SELECT id, user_id, name, currency, is_active, created_at
FROM accounts
WHERE id = $1;

-- name: DeactivateAccount :exec
UPDATE accounts SET is_active = FALSE WHERE id = $1;
