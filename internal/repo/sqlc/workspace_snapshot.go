package repo

import (
	"context"
	"encoding/json"
	"github.com/EftikharAzim/ledgerx/internal/workspace"
)

// ReadWorkspaceData reads one owner-scoped snapshot, explicitly excluding auth,
// idempotency keys, preview payloads, jobs, storage paths, and derived caches.
func (q *Queries) ReadWorkspaceData(ctx context.Context, uid int64) (workspace.Data, error) {
	var data workspace.Data
	var transactions, postings, plans, budgets int64
	if err := q.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM transactions WHERE user_id=$1),
 (SELECT count(*) FROM postings p JOIN transactions t ON t.id=p.transaction_id WHERE t.user_id=$1), (SELECT count(*) FROM planned_transactions WHERE user_id=$1), (SELECT count(*) FROM budgets WHERE user_id=$1)`, uid).Scan(&transactions, &postings, &plans, &budgets); err != nil {
		return data, err
	}
	if transactions > workspace.MaxTransactions || postings > workspace.MaxPostings || plans > 50000 || budgets > 10000 {
		return data, workspace.ErrLimits
	}
	const snapshot = `SELECT jsonb_build_object(
 'accounts',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT id,name,currency,is_active AS active,kind,created_at,tracking_start_at FROM accounts WHERE user_id=$1) x),
 'categories',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT id,name,created_at,COALESCE(parent_id,0) AS parent_id,icon,kind,archived,deleted,deleted_at,COALESCE(replacement_id,0) AS replacement_id FROM categories WHERE user_id=$1) x),
 'transactions',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT id,currency,COALESCE(category_id,0) AS category_id,COALESCE(suggested_category_id,0) AS suggested_category_id,suggestion_score,occurred_at,created_at,COALESCE(note,'') AS note,COALESCE(reversal_of,0) AS reversal_of,entry_type,exclude_from_reports FROM transactions WHERE user_id=$1) x),
 'postings',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT p.id,p.transaction_id,p.account_id,p.amount_minor,p.created_at FROM postings p JOIN transactions t ON t.id=p.transaction_id WHERE t.user_id=$1) x),
 'imports',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT id,account_id,filename,file_hash,created_at,completed_at,result AS receipt FROM import_sessions WHERE user_id=$1 AND status='completed') x),
 'origins',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT id,account_id,import_id,transaction_id,row_number,fingerprint,source_ref,date_only FROM imported_transactions WHERE user_id=$1) x),
 'corrections',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT id,original_id,reversal_id,COALESCE(replacement_id,0) AS replacement_id,COALESCE(import_source_id,0) AS import_source_id,date_only,receipt,created_at FROM transaction_corrections WHERE user_id=$1) x),
 'planned_transactions',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT id,account_id,currency,amount_minor,COALESCE(category_id,0) AS category_id,occurred_at,note,entry_type,date_only,exclude_from_reports,status,COALESCE(transaction_id,0) AS transaction_id,COALESCE(import_id,0) AS import_id,COALESCE(row_number,0) AS row_number,fingerprint,source_ref,created_at FROM planned_transactions WHERE user_id=$1) x),
 'budgets',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT id,currency,to_char(month,'YYYY-MM') AS month,COALESCE(category_id,0) AS category_id,amount_minor,created_at,updated_at FROM budgets WHERE user_id=$1) x),
 'balance_checks',(SELECT COALESCE(jsonb_agg(x ORDER BY x.id),'[]'::jsonb) FROM (SELECT id,account_id,actual_minor,recorded_minor,difference_minor,ledger_revision,note,created_at FROM balance_checks WHERE user_id=$1) x)
 )`
	var raw []byte
	if err := q.db.QueryRow(ctx, snapshot, uid).Scan(&raw); err != nil {
		return data, err
	}
	if len(raw) > workspace.MaxBytes {
		return data, workspace.ErrLimits
	}
	err := json.Unmarshal(raw, &data)
	return data, err
}
