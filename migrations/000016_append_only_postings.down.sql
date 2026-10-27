DROP TRIGGER transactions_append_only_delete ON transactions;
DROP FUNCTION protect_transaction_deletion();
DROP TRIGGER postings_append_only ON postings;
DROP FUNCTION protect_posting_mutation();
ALTER TABLE postings ALTER CONSTRAINT postings_account_id_fkey NOT DEFERRABLE INITIALLY IMMEDIATE;
ALTER TABLE transactions ALTER CONSTRAINT transactions_reversal_of_fkey NOT DEFERRABLE INITIALLY IMMEDIATE;
