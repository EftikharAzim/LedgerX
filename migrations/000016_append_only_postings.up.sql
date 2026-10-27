-- Posting corrections must use reversal transactions. Keep the existing
-- user-deletion cascade for retention cleanup, while rejecting direct ledger
-- mutations and direct transaction deletion.
-- User deletion cascades through accounts and transactions in trigger order.
-- Check these NO ACTION references after the complete cascade has finished.
ALTER TABLE postings ALTER CONSTRAINT postings_account_id_fkey DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE transactions ALTER CONSTRAINT transactions_reversal_of_fkey DEFERRABLE INITIALLY DEFERRED;

CREATE FUNCTION protect_posting_mutation() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'DELETE' AND NOT EXISTS (
    SELECT 1 FROM transactions WHERE id = OLD.transaction_id
  ) THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION 'postings are append-only; use a reversal'
    USING ERRCODE = '55000';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER postings_append_only
  BEFORE UPDATE OR DELETE ON postings
  FOR EACH ROW EXECUTE FUNCTION protect_posting_mutation();

CREATE FUNCTION protect_transaction_deletion() RETURNS trigger AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM users WHERE id = OLD.user_id) THEN
    RAISE EXCEPTION 'transactions are append-only; delete the owning user for retention cleanup'
      USING ERRCODE = '55000';
  END IF;
  RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER transactions_append_only_delete
  BEFORE DELETE ON transactions
  FOR EACH ROW EXECUTE FUNCTION protect_transaction_deletion();
