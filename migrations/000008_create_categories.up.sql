CREATE TABLE categories (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id), -- categories are usually user-specific
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);
