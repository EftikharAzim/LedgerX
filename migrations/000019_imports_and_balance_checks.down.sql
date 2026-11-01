DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM imported_transactions) OR EXISTS (SELECT 1 FROM balance_checks) THEN
    RAISE EXCEPTION 'cannot remove import provenance or balance checks while records exist';
  END IF;
END $$;
DROP TABLE balance_checks;
DROP TABLE imported_transactions;
DROP TABLE import_sessions;
ALTER TABLE accounts DROP COLUMN tracking_start_at;
