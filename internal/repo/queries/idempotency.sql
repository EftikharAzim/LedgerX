-- name: UpsertIdempotencyStart :one
INSERT INTO idempotency_keys (user_id, key, request_hash, status)
VALUES ($1, $2, $3, 'started')
ON CONFLICT (user_id, key) DO UPDATE
SET request_hash = EXCLUDED.request_hash
RETURNING user_id, key, request_hash, status, response_json, created_at;

-- name: MarkIdempotencySuccess :exec
UPDATE idempotency_keys
SET status = 'succeeded', response_json = $4
WHERE user_id = $1 AND key = $2 AND request_hash = $3;

-- name: GetIdempotency :one
SELECT user_id, key, request_hash, status, response_json, created_at
FROM idempotency_keys
WHERE user_id = $1 AND key = $2;
