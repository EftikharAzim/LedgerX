-- name: CreateCategory :one
INSERT INTO categories (user_id, name)
VALUES ($1, $2)
RETURNING id, user_id, name, created_at;

-- name: ListCategoriesByUser :many
SELECT id, user_id, name, created_at
FROM categories
WHERE user_id = $1
ORDER BY name;

-- name: GetCategoryForUser :one
SELECT id, user_id, name, created_at
FROM categories
WHERE id = $1 AND user_id = $2;
