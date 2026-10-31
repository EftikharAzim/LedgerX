-- Refuse to discard the date-range metadata of existing non-monthly reports.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM exports WHERE report_type <> 'monthly') THEN
        RAISE EXCEPTION 'Cannot roll back export periods while non-monthly reports exist';
    END IF;
END $$;

ALTER TABLE exports DROP CONSTRAINT exports_period_check,
    ALTER COLUMN month SET NOT NULL,
    DROP COLUMN report_type,
    DROP COLUMN start_date,
    DROP COLUMN end_date;
