ALTER TABLE exports
    ALTER COLUMN month DROP NOT NULL,
    ADD COLUMN report_type TEXT NOT NULL DEFAULT 'monthly',
    ADD COLUMN start_date DATE,
    ADD COLUMN end_date DATE;

UPDATE exports
SET start_date = month,
    end_date = (month + INTERVAL '1 month' - INTERVAL '1 day')::date;

ALTER TABLE exports ADD CONSTRAINT exports_period_check CHECK (
    (report_type = 'all' AND month IS NULL AND start_date IS NULL AND end_date IS NULL)
    OR
    (report_type = 'monthly' AND month IS NOT NULL
      AND month = date_trunc('month', month)::date
      AND start_date IS NOT NULL AND start_date = month
      AND end_date IS NOT NULL AND end_date = (month + INTERVAL '1 month' - INTERVAL '1 day')::date)
    OR
    (report_type = 'weekly' AND month IS NULL
      AND start_date IS NOT NULL AND end_date IS NOT NULL
      AND EXTRACT(ISODOW FROM start_date) = 1 AND end_date = start_date + 6)
    OR
    (report_type = 'yearly' AND month IS NULL
      AND start_date IS NOT NULL AND end_date IS NOT NULL
      AND start_date = date_trunc('year', start_date)::date
      AND end_date = (start_date + INTERVAL '1 year' - INTERVAL '1 day')::date)
    OR
    (report_type = 'custom' AND month IS NULL
      AND start_date IS NOT NULL AND end_date IS NOT NULL AND end_date >= start_date)
);
