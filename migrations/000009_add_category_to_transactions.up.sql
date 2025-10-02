ALTER TABLE transactions
ADD COLUMN category_id BIGINT REFERENCES categories(id);
