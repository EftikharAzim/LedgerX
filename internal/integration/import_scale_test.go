package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
)

func TestBulkDuplicateReviewWithTenThousandExistingRecords(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var external, session, firstID int64
	if err := tx.QueryRow(ctx, "INSERT INTO accounts(user_id,name,currency,kind) VALUES ($1,'External','USD','external') RETURNING id", uid).Scan(&external); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO transactions(user_id,currency,occurred_at,note) SELECT $1,'USD','2026-01-02T00:00:00Z','Synthetic history ' || g FROM generate_series(1,10000) g", uid); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO postings(transaction_id,account_id,amount_minor) SELECT id,$2::bigint,-100 FROM transactions WHERE user_id=$1 UNION ALL SELECT id,$3::bigint,100 FROM transactions WHERE user_id=$1", uid, account, external); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, "INSERT INTO import_sessions(user_id,account_id,filename,file_hash,status,preview,commit_hash,result,completed_at) VALUES ($1,$2,'synthetic-history.csv','fixture','completed','{}','fixture','{}',now()) RETURNING id", uid, account).Scan(&session); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO imported_transactions(user_id,account_id,import_id,transaction_id,row_number,fingerprint,source_ref) SELECT $1,$2,$3,id,(row_number() OVER(ORDER BY id))::integer,'fingerprint-'||id,'existing-'||id FROM transactions WHERE user_id=$1", uid, account, session); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, "SELECT min(id) FROM transactions WHERE user_id=$1", uid).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// This fixture models an existing history. Bulk SQL bypasses the normal
	// autovacuum/analyze cycle; establish its statistics before timing reads.
	if _, err := pool.Exec(ctx, "ANALYZE accounts; ANALYZE transactions; ANALYZE postings; ANALYZE imported_transactions; ANALYZE transaction_corrections;"); err != nil {
		t.Fatal(err)
	}
	input := sqlc.FindImportDuplicatesParams{UserID: uid, AccountID: account}
	for i := range 1000 {
		input.RowNumbers = append(input.RowNumbers, int32(i+2))
		input.OccurredAts = append(input.OccurredAts, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
		input.Amounts = append(input.Amounts, -100)
		input.EntryTypes = append(input.EntryTypes, "standard")
		input.Fingerprints = append(input.Fingerprints, fmt.Sprintf("new-fingerprint-%d", i))
		input.SourceRefs = append(input.SourceRefs, fmt.Sprintf("new-reference-%d", i))
	}
	// One real reference match among 999 new purchases with the same amount/date.
	// The deadline stays below the API's existing ten-second request budget.
	input.SourceRefs[0] = fmt.Sprintf("existing-%d", firstID)
	bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	rows, err := q.FindImportDuplicates(bounded, input)
	if err != nil {
		t.Fatal("bulk review exceeded its request budget or failed", err)
	}
	if len(rows) != 1 || rows[0].RowNumber != 2 || rows[0].TransactionID != firstID || rows[0].MatchType != "reference" {
		t.Fatalf("bulk reference review changed: matches=%d", len(rows))
	}
}
