DO $$ BEGIN IF EXISTS(SELECT 1 FROM recurring_schedules) OR EXISTS(SELECT 1 FROM planned_transactions WHERE recorded_amount_minor IS NOT NULL AND recorded_amount_minor<>amount_minor) THEN RAISE EXCEPTION 'cannot remove recurring plans with data; back up before downgrading'; END IF; END $$;
ALTER TABLE planned_transactions DROP COLUMN schedule_id,DROP COLUMN scheduled_date,DROP COLUMN schedule_revision,DROP COLUMN cancellation_reason,DROP COLUMN recorded_amount_minor;
DROP TABLE recurring_schedules;
