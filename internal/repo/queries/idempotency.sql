-- Claim an idempotency key. Returns no row when the key already exists;
-- callers then read the existing record with GetIdempotency.
-- name: InsertIdempotencyKey :one
INSERT INTO idempotency_keys (user_id, key, request_hash, status)
VALUES ($1, $2, $3, 'started')
ON CONFLICT (user_id, key) DO NOTHING
RETURNING user_id, key, request_hash, status, response_json, created_at;

-- name: GetIdempotency :one
SELECT user_id, key, request_hash, status, response_json, created_at
FROM idempotency_keys
WHERE user_id = $1 AND key = $2;

-- name: MarkIdempotencySuccess :exec
UPDATE idempotency_keys
SET status = 'succeeded', response_json = $4
WHERE user_id = $1 AND key = $2 AND request_hash = $3;
