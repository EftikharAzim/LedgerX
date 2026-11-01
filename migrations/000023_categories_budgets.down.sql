DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM budgets) OR EXISTS(SELECT 1 FROM categories WHERE parent_id IS NOT NULL OR archived OR deleted OR icon<>'tag' OR kind<>'any') THEN
  RAISE EXCEPTION 'cannot remove category organization or budgets with data; back up before downgrading';
 END IF;
END $$;
DROP TABLE budgets;
DROP INDEX categories_user_lower_name;
CREATE UNIQUE INDEX categories_user_lower_name ON categories(user_id,lower(name));
ALTER TABLE categories DROP COLUMN parent_id, DROP COLUMN icon, DROP COLUMN kind,
 DROP COLUMN archived, DROP COLUMN deleted, DROP COLUMN deleted_at, DROP COLUMN replacement_id;
