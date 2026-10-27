-- Refresh tokens enable short-lived access JWTs plus server-side revocation
-- (logout, theft response). The opaque token is never stored — only its
-- SHA-256 hash — so a database leak does not yield usable tokens.
CREATE TABLE refresh_tokens (
  id          BIGSERIAL PRIMARY KEY,
  user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  TEXT   NOT NULL UNIQUE,    -- sha256(hex) of the opaque token
  expires_at  TIMESTAMPTZ NOT NULL,
  revoked_at  TIMESTAMPTZ,               -- non-null once rotated or logged out
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
