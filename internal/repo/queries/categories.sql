-- name: CreateCategory :one
INSERT INTO categories (user_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: ListCategoriesByUser :many
SELECT *
FROM categories
WHERE user_id = $1 AND NOT deleted
ORDER BY name;

-- name: GetCategoryForUser :one
SELECT *
FROM categories
WHERE id = $1 AND user_id = $2;

-- name: ListSelectableCategoriesByUser :many
SELECT c.* FROM categories c
LEFT JOIN categories parent ON parent.id=c.parent_id
WHERE c.user_id=$1 AND NOT c.archived AND NOT c.deleted
 AND (c.parent_id IS NULL OR (NOT parent.archived AND NOT parent.deleted)) ORDER BY c.name;
