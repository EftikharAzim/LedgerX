-- name: CreateUser :one
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING id, email, password_hash, created_at;

-- name: ListUsers :many
SELECT id, email, password_hash, created_at
FROM users
ORDER BY id
LIMIT $1 OFFSET $2;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, created_at
FROM users
WHERE email = $1;
