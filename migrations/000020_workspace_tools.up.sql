CREATE TABLE transaction_corrections (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  original_id BIGINT NOT NULL UNIQUE REFERENCES transactions(id) ON DELETE CASCADE,
  reversal_id BIGINT NOT NULL UNIQUE REFERENCES transactions(id) ON DELETE CASCADE,
  replacement_id BIGINT UNIQUE REFERENCES transactions(id) ON DELETE CASCADE,
  import_source_id BIGINT REFERENCES transactions(id) ON DELETE CASCADE,
  date_only BOOLEAN NOT NULL DEFAULT FALSE,
  request_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  receipt JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(user_id, request_key)
);

CREATE TABLE workspace_restores (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  checksum TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'preview' CHECK(status IN ('preview', 'completed')),
  payload JSONB NOT NULL,
  summary JSONB NOT NULL,
  result JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '24 hours',
  completed_at TIMESTAMPTZ,
  CHECK ((status = 'completed') = (result IS NOT NULL AND completed_at IS NOT NULL))
);
CREATE INDEX workspace_restores_user_id ON workspace_restores(user_id, id DESC);
