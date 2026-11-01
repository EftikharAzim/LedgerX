DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM planned_transactions) OR EXISTS (SELECT 1 FROM transactions WHERE exclude_from_reports) THEN
    RAISE EXCEPTION 'cannot remove planned activity or report exclusions; back up and resolve these records before downgrading';
  END IF;
END $$;
DROP TABLE planned_transactions;
ALTER TABLE transactions DROP COLUMN exclude_from_reports;
