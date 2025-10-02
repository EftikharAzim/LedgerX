ALTER TABLE users
ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';

-- optional: unique constraint for email
ALTER TABLE users
ADD CONSTRAINT users_email_unique UNIQUE (email);
