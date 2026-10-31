-- Dropping classification would turn starting balances/refunds into earnings.
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM transactions WHERE entry_type <> 'standard') THEN
    RAISE EXCEPTION 'cannot remove spending types while classified transactions exist';
  END IF;
END $$;
DROP INDEX transactions_user_occurred_id;
DROP INDEX accounts_creation_key;
ALTER TABLE accounts DROP CONSTRAINT accounts_creation_identity,
  DROP COLUMN creation_key, DROP COLUMN creation_hash;
ALTER TABLE transactions DROP COLUMN entry_type;
