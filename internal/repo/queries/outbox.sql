-- name: CreateOutboxEvent :one
INSERT INTO outbox (event_type, payload)
VALUES ($1, $2)
RETURNING id, created_at;
