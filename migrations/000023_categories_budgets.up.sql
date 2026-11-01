ALTER TABLE categories
  ADD COLUMN parent_id BIGINT REFERENCES categories(id) DEFERRABLE INITIALLY DEFERRED,
  ADD COLUMN icon TEXT NOT NULL DEFAULT 'tag',
  ADD COLUMN kind TEXT NOT NULL DEFAULT 'any' CHECK (kind IN ('any','income','expense')),
  ADD COLUMN archived BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN deleted BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN deleted_at TIMESTAMPTZ,
  ADD COLUMN replacement_id BIGINT REFERENCES categories(id) DEFERRABLE INITIALLY DEFERRED,
  ADD CHECK (parent_id IS DISTINCT FROM id),
  ADD CHECK (replacement_id IS DISTINCT FROM id),
  ADD CHECK (deleted = (deleted_at IS NOT NULL));
DROP INDEX categories_user_lower_name;
CREATE UNIQUE INDEX categories_user_lower_name ON categories(user_id,lower(name)) WHERE NOT deleted;
CREATE INDEX categories_parent ON categories(user_id,parent_id);

CREATE TABLE budgets (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  currency TEXT NOT NULL,
  month DATE NOT NULL CHECK (extract(day FROM month)=1),
  category_id BIGINT REFERENCES categories(id) DEFERRABLE INITIALLY DEFERRED,
  amount_minor BIGINT NOT NULL CHECK (amount_minor>0 AND amount_minor<=9007199254740991),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX budgets_scope ON budgets(user_id,currency,month,COALESCE(category_id,0));
