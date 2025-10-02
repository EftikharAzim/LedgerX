CREATE INDEX IF NOT EXISTS idx_tx_account_time
ON transactions(account_id, occurred_at);
