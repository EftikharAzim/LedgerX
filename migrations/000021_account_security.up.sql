ALTER TABLE users ADD COLUMN auth_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN recovery_code_hash TEXT;

-- Complete the owning-user retention cascade without allowing ordinary
-- transaction or posting deletion. Category references are checked at commit.
ALTER TABLE exports DROP CONSTRAINT exports_user_id_fkey;
ALTER TABLE exports ADD CONSTRAINT exports_user_id_fkey
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE categories DROP CONSTRAINT categories_user_id_fkey;
ALTER TABLE categories ADD CONSTRAINT categories_user_id_fkey
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE transactions ALTER CONSTRAINT transactions_category_id_fkey DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE transactions ALTER CONSTRAINT transactions_suggested_category_id_fkey DEFERRABLE INITIALLY DEFERRED;

-- Survives deletion of the user and process failure. Contains only opaque
-- server-generated object keys; files are retried until deletion succeeds.
CREATE TABLE export_deletions (
  file_key TEXT PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE privacy_deletions (
  user_id BIGINT PRIMARY KEY,
  stream_cursor TEXT NOT NULL DEFAULT '-',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
