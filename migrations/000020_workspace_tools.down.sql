DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM transaction_corrections) OR EXISTS (SELECT 1 FROM workspace_restores WHERE status='completed') THEN
    RAISE EXCEPTION 'cannot remove correction history or completed restore receipts';
  END IF;
END $$;
DROP TABLE workspace_restores;
DROP TABLE transaction_corrections;
