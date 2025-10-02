CREATE TABLE exports (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    month DATE NOT NULL, -- first day of month, e.g. 2025-09-01
    status TEXT NOT NULL DEFAULT 'pending', -- pending, processing, done, error
    file_path TEXT, -- where CSV is stored
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);