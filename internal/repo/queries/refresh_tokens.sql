-- name: InsertRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, user_id, token_hash, expires_at, revoked_at, created_at;

-- Consume the old token and insert its successor in one statement. A second
-- rotation waits for the first update, then returns no rows. If insertion
-- fails, PostgreSQL rolls back the consumption as well.
-- name: RotateRefreshToken :one
WITH consumed AS (
  UPDATE refresh_tokens AS old
  SET revoked_at = now()
  WHERE old.token_hash = sqlc.arg(old_token_hash)
    AND old.revoked_at IS NULL
    AND old.expires_at > now()
  RETURNING old.user_id
)
INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
SELECT consumed.user_id, sqlc.arg(new_token_hash), sqlc.arg(expires_at)
FROM consumed
RETURNING id, user_id, token_hash, expires_at, revoked_at, created_at;

-- name: GetRefreshToken :one
SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
FROM refresh_tokens
WHERE token_hash = $1;

-- Serialize token issuance, rotation, logout, and reuse revocation per user.
-- Call in a READ COMMITTED transaction before rereading token state.
-- name: LockAuthUser :one
SELECT id FROM users WHERE id = $1 FOR UPDATE;

-- Revoke a single token only if currently active. rows_affected lets the
-- caller distinguish "revoked it" from "was already revoked".
-- name: RevokeRefreshToken :execrows
UPDATE refresh_tokens
SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- Theft response: kill every active token for a user.
-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL;
