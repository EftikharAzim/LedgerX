-- All financial writers share this per-user lock. Imports can recheck duplicates
-- and commit a complete batch without racing an ordinary transaction or reversal.
-- name: LockUserLedger :exec
SELECT pg_advisory_xact_lock(hashtextextended('ledger-user:' || sqlc.arg(user_id)::bigint::text, 0));

-- name: GetAccountLedgerContext :one
SELECT a.id, a.name, a.currency, a.is_active, a.tracking_start_at,
  COALESCE(l.balance_minor, 0)::bigint AS balance_minor,
  COALESCE(l.revision, 0)::bigint AS revision,
  COALESCE(l.entry_count, 0)::bigint AS entry_count,
  COALESCE(o.id, 0)::bigint AS opening_id,
  COALESCE(o.amount_minor, 0)::bigint AS opening_minor,
  COALESCE(o.occurred_at, '0001-01-01T00:00:00Z'::timestamptz)::timestamptz AS opening_at,
  (SELECT COUNT(*) FROM transactions ot JOIN postings op ON op.transaction_id = ot.id
    WHERE op.account_id = a.id AND ot.entry_type = 'opening_balance' AND ot.reversal_of IS NULL
      AND NOT EXISTS (SELECT 1 FROM transactions r WHERE r.reversal_of = ot.id))::bigint AS opening_count
FROM accounts a
LEFT JOIN LATERAL (
  SELECT COALESCE(SUM(p.amount_minor), 0)::bigint AS balance_minor,
    COALESCE(MAX(p.id), 0)::bigint AS revision, COUNT(*)::bigint AS entry_count
  FROM postings p WHERE p.account_id = a.id
) l ON true
LEFT JOIN LATERAL (
  SELECT t.id, p.amount_minor, t.occurred_at FROM transactions t
  JOIN postings p ON p.transaction_id = t.id
  WHERE p.account_id = a.id AND t.entry_type = 'opening_balance' AND t.reversal_of IS NULL
    AND NOT EXISTS (SELECT 1 FROM transactions r WHERE r.reversal_of = t.id)
  ORDER BY t.id DESC LIMIT 1
) o ON true
WHERE a.id = sqlc.arg(account_id) AND a.user_id = sqlc.arg(user_id) AND a.kind = 'normal';

-- name: SetTrackingStart :exec
UPDATE accounts SET tracking_start_at = sqlc.narg(tracking_start_at)::timestamptz WHERE id = sqlc.arg(account_id);

-- name: EnsureImportCategory :one
INSERT INTO categories (user_id, name) VALUES ($1, $2)
ON CONFLICT (user_id, lower(name)) WHERE NOT deleted DO UPDATE SET name = categories.name
RETURNING id, name;

-- name: CreateImportSession :one
INSERT INTO import_sessions (user_id, account_id, filename, file_hash, preview)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetImportSession :one
SELECT * FROM import_sessions WHERE id = $1 AND user_id = $2;

-- name: GetImportSessionForUpdate :one
SELECT * FROM import_sessions WHERE id = $1 AND user_id = $2 FOR UPDATE;

-- name: CompleteImportSession :exec
UPDATE import_sessions SET status = 'completed', commit_hash = $3, result = $4,
  completed_at = now(), preview = '{}'::jsonb
WHERE id = $1 AND user_id = $2;

-- name: ListImportSessions :many
SELECT id, account_id, filename, status, created_at, completed_at, result
FROM import_sessions WHERE user_id = $1 ORDER BY id DESC LIMIT 10;

-- name: DeleteExpiredImportPreviews :exec
DELETE FROM import_sessions WHERE user_id = $1 AND status = 'preview' AND expires_at < now();

-- name: PruneImportPreviews :exec
DELETE FROM import_sessions AS old WHERE old.user_id = $1 AND old.status = 'preview'
  AND old.id NOT IN (SELECT recent.id FROM import_sessions AS recent WHERE recent.user_id = $1 AND recent.status = 'preview' ORDER BY recent.id DESC LIMIT 10);

-- name: CreateImportedTransaction :exec
INSERT INTO imported_transactions (user_id, account_id, import_id, transaction_id, row_number, fingerprint, source_ref, date_only)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- One bounded candidate list per requested row; same-day same-amount matches
-- are intentionally only possible duplicates, never automatic deletions.
-- name: FindImportDuplicates :many
WITH requested AS MATERIALIZED (
  SELECT unnest(sqlc.arg(row_numbers)::integer[]) AS row_number,
    unnest(sqlc.arg(occurred_ats)::timestamptz[]) AS occurred_at,
    unnest(sqlc.arg(amounts)::bigint[]) AS amount_minor,
    unnest(sqlc.arg(entry_types)::text[]) AS entry_type,
    unnest(sqlc.arg(fingerprints)::text[]) AS fingerprint,
    unnest(sqlc.arg(source_refs)::text[]) AS source_ref,
    ((unnest(sqlc.arg(occurred_ats)::timestamptz[])) AT TIME ZONE 'UTC')::date AS day
), candidates AS MATERIALIZED (
  -- Resolve ownership, correction provenance, and active ledger entries once
  -- per batch, rather than repeating these joins for every requested CSV row.
  SELECT t.id, p.id AS posting_id, t.occurred_at, (t.occurred_at AT TIME ZONE 'UTC')::date AS day, t.entry_type, p.amount_minor,
    COALESCE(t.note, '')::text AS note, current_account.name AS account_name,
    p.account_id AS current_account_id, it.account_id AS import_account_id,
    it.fingerprint, it.source_ref
  FROM postings p JOIN transactions t ON t.id = p.transaction_id
  JOIN accounts current_account ON current_account.id=p.account_id AND current_account.kind='normal'
  LEFT JOIN transaction_corrections correction ON correction.replacement_id=t.id
  LEFT JOIN imported_transactions it ON it.transaction_id = COALESCE(correction.import_source_id,t.id)
  WHERE (p.account_id = sqlc.arg(account_id) OR it.account_id=sqlc.arg(account_id)) AND t.user_id = sqlc.arg(user_id)
    AND t.reversal_of IS NULL AND t.entry_type <> 'opening_balance'
    AND NOT EXISTS (SELECT 1 FROM transactions r WHERE r.reversal_of = t.id)
    AND EXISTS (SELECT 1 FROM postings ep JOIN accounts ea ON ea.id = ep.account_id
      WHERE ep.transaction_id = t.id AND ea.kind = 'external')
)
, matched_ids AS (
  SELECT requested.row_number, candidates.posting_id
  FROM requested JOIN candidates ON candidates.fingerprint = requested.fingerprint
  WHERE candidates.import_account_id=sqlc.arg(account_id)
  UNION
  SELECT requested.row_number, candidates.posting_id
  FROM requested JOIN candidates ON candidates.source_ref = requested.source_ref
  WHERE candidates.import_account_id=sqlc.arg(account_id) AND requested.source_ref <> ''
  UNION
  SELECT requested.row_number, candidates.posting_id
  FROM requested JOIN candidates ON candidates.amount_minor=requested.amount_minor
    AND candidates.entry_type=requested.entry_type AND candidates.day=requested.day
  WHERE candidates.current_account_id=sqlc.arg(account_id) AND requested.source_ref = ''
  UNION
  SELECT requested.row_number, candidates.posting_id
  FROM requested JOIN candidates ON candidates.amount_minor=requested.amount_minor
    AND candidates.entry_type=requested.entry_type AND candidates.day=requested.day
  WHERE candidates.current_account_id=sqlc.arg(account_id) AND requested.source_ref <> ''
    AND (candidates.import_account_id IS NULL OR candidates.import_account_id<>sqlc.arg(account_id)
      OR candidates.source_ref IS NULL OR candidates.source_ref = '')
), matches AS (
  SELECT requested.row_number, candidates.id, candidates.occurred_at,
    candidates.amount_minor, candidates.note, candidates.account_name,
    CASE WHEN candidates.import_account_id=sqlc.arg(account_id) AND requested.source_ref <> '' AND candidates.source_ref = requested.source_ref THEN 'reference'
      WHEN candidates.import_account_id=sqlc.arg(account_id) AND candidates.fingerprint = requested.fingerprint THEN 'exact'
      ELSE 'date_amount' END::text AS match_type,
    row_number() OVER (PARTITION BY requested.row_number ORDER BY
      (candidates.import_account_id=sqlc.arg(account_id) AND requested.source_ref <> '' AND candidates.source_ref = requested.source_ref) DESC NULLS LAST,
      (candidates.import_account_id=sqlc.arg(account_id) AND candidates.fingerprint = requested.fingerprint) DESC NULLS LAST,
      candidates.id DESC) AS duplicate_rank
  FROM matched_ids JOIN requested ON requested.row_number=matched_ids.row_number
  JOIN candidates ON candidates.posting_id=matched_ids.posting_id
)
SELECT matches.row_number::integer AS row_number, matches.id AS transaction_id,
  matches.occurred_at, matches.amount_minor, matches.note, matches.match_type, matches.account_name
FROM matches WHERE duplicate_rank <= 3
ORDER BY matches.row_number, matches.id;

-- name: CreateBalanceCheck :one
INSERT INTO balance_checks (user_id, account_id, actual_minor, recorded_minor, difference_minor, ledger_revision, note, request_key, request_hash)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;

-- name: GetBalanceCheckByKey :one
SELECT * FROM balance_checks WHERE user_id = $1 AND request_key = $2;

-- name: ListBalanceChecks :many
SELECT * FROM balance_checks WHERE user_id = $1 AND account_id = $2 ORDER BY id DESC LIMIT 5;
