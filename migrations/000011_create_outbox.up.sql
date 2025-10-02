CREATE TABLE outbox (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ
);

-- Index to find unprocessed events quickly
CREATE INDEX idx_outbox_unprocessed ON outbox (processed_at) WHERE processed_at IS NULL;
