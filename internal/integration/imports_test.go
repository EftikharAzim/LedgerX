package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	"github.com/EftikharAzim/ledgerx/internal/money"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/go-chi/chi/v5"
)

func importColumn(i int) *int { return &i }
func importInput(account int64, csv string) service.ImportInput {
	return service.ImportInput{AccountID: account, Filename: "bank.csv", CSV: csv, Delimiter: "auto", Mapping: importing.Mapping{Date: importColumn(0), Amount: importColumn(1), Note: importColumn(2), Category: importColumn(3), Reference: importColumn(4), AmountMode: "signed", DateFormat: "iso", NumberStyle: "dot", SignConvention: "income_positive"}, Starting: service.StartingBalancePlan{Mode: "existing"}}
}
func importReady(v service.ImportView) service.ImportSelection {
	selection := service.ImportSelection{}
	for _, r := range v.Preview.Rows {
		if len(r.Errors) == 0 && len(r.Duplicates) == 0 {
			selection.Rows = append(selection.Rows, r.Number)
		}
	}
	return selection
}
func importPreview(t *testing.T, uid int64, in service.ImportInput) service.ImportView {
	t.Helper()
	v, err := service.NewImportService(pool, q, sumSvc).Preview(context.Background(), uid, in)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func importCommit(t *testing.T, uid int64, v service.ImportView) service.ImportReceipt {
	t.Helper()
	r, err := service.NewImportService(pool, q, sumSvc).Commit(context.Background(), uid, v.ID, importReady(v))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func importContext(t *testing.T, uid, account int64) service.AccountLedgerContext {
	t.Helper()
	v, err := service.NewImportService(pool, q, sumSvc).Context(context.Background(), uid, account)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func importsRouter(uid int64) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), httptransport.UserIDKey, uid)))
		})
	})
	httptransport.NewImportsAPI(q, service.NewImportService(pool, q, sumSvc), service.NewBalanceCheckService(pool, q)).Routes(r)
	httptransport.NewSpendingAPI(q).Routes(r)
	httptransport.NewTransactionsAPI(q, txSvc).Routes(r)
	return r
}
func importRequest(t *testing.T, uid int64, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	importsRouter(uid).ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	if w.Code != status {
		t.Fatalf("%s %s: %d %s; want %d", method, path, w.Code, w.Body.String(), status)
	}
	return w
}

func TestImportPreviewCommitAndProvenance(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	in := importInput(account, "Date,Amount,Description,Category,Reference\n2024-01-02,-12.34,Coffee,Food,A\n2024-01-03,100,Salary,Income,B\ninvalid,1,Invalid,Unwanted,C\n2024-01-02,-12.34,Coffee,Food,A")
	in.Starting = service.StartingBalancePlan{Mode: "set", AmountMinor: 5000}
	v := importPreview(t, uid, in)
	if v.Preview.ReadyCount != 2 || v.Preview.DuplicateCount != 1 || v.Preview.InvalidCount != 1 || v.Preview.ProjectedMinor != 13766 || v.Preview.Starting.Date != "2024-01-02" {
		t.Fatalf("preview %+v", v.Preview)
	}
	before := importContext(t, uid, account)
	cats, err := q.ListCategoriesByUser(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if before.EntryCount != 0 || before.BalanceMinor != 0 || len(cats) != 0 {
		t.Fatalf("preview changed ledger: %+v, %+v", before, cats)
	}
	// A caller cannot select invalid rows or approve duplicates implicitly.
	svc := service.NewImportService(pool, q, sumSvc)
	for _, selection := range []service.ImportSelection{{Rows: []int{4}}, {Rows: []int{2, 5}}, {Rows: []int{2}, DuplicateApprovals: []int{2}}, {Rows: []int{999}}} {
		if _, err := svc.Commit(ctx, uid, v.ID, selection); !errors.Is(err, service.ErrImportSelection) {
			t.Fatalf("selection %+v: %v", selection, err)
		}
	}
	receipt := importCommit(t, uid, v)
	if receipt.ImportedCount != 2 || receipt.SkippedCount != 2 || receipt.InvalidCount != 1 || receipt.DuplicateSkipped != 1 || receipt.NewCategories != 2 || receipt.BalanceAfter != 13766 {
		t.Fatalf("receipt %+v", receipt)
	}
	after := importContext(t, uid, account)
	if after.BalanceMinor != 13766 || after.EntryCount != 3 || after.OpeningMinor != 5000 {
		t.Fatalf("context %+v", after)
	}
	saved, err := svc.Get(ctx, uid, v.ID)
	if err != nil || saved.Preview != nil || saved.Receipt == nil || saved.Receipt.ID != receipt.ID {
		t.Fatalf("resume %+v %v", saved, err)
	}
	var payload struct {
		Entries []struct {
			ImportID int64 `json:"import_id"`
			DateOnly bool  `json:"import_date_only"`
			ID       int64 `json:"transaction_id"`
		} `json:"entries"`
	}
	getWorkspace(t, importsRouter(uid), fmt.Sprintf("/activity?import_id=%d", v.ID), &payload)
	if len(payload.Entries) != 2 {
		t.Fatalf("activity %+v", payload)
	}
	for _, e := range payload.Entries {
		if e.ImportID != v.ID || !e.DateOnly {
			t.Fatalf("missing provenance %+v", e)
		}
	}
	var summary map[string]any
	getWorkspace(t, importsRouter(uid), "/spending?type=all&currency=USD", &summary)
	if summary["income_minor"] != float64(10000) || summary["spending_minor"] != float64(1234) {
		t.Fatalf("opening counted as spending/income: %+v", summary)
	}
	rows, err := q.ListPostingsForExport(ctx, sqlc.ListPostingsForExportParams{UserID: uid})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, r := range rows {
		if r.ImportID.Valid {
			count++
			if !r.DateOnly || r.ImportID.Int64 != v.ID {
				t.Fatalf("export origin %+v", r)
			}
		}
	}
	if count != 2 {
		t.Fatalf("export imported rows=%d", count)
	}
	repeated := importPreview(t, uid, in)
	if repeated.Preview.ReadyCount != 0 || repeated.Preview.DuplicateCount != 3 {
		t.Fatalf("reimport %+v", repeated.Preview)
	}
}

func TestImportDuplicateReviewAndBankReferences(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	svc := service.NewImportService(pool, q, sumSvc)
	in := importInput(account, "Date,Amount,Description,Category,Reference\n2024-01-02,-1,Coffee,Food,A\n2024-01-02,-1,Coffee,Food,B\n2024-01-02,-1,Coffee,Food,")
	v := importPreview(t, uid, in)
	if v.Preview.ReadyCount != 2 || v.Preview.DuplicateCount != 1 {
		t.Fatalf("distinct bank refs not distinguished %+v", v.Preview)
	}
	if _, err := svc.Commit(ctx, uid, v.ID, service.ImportSelection{Rows: []int{2, 3, 4}}); !errors.Is(err, service.ErrImportSelection) {
		t.Fatal(err)
	}
	r, err := svc.Commit(ctx, uid, v.ID, service.ImportSelection{Rows: []int{4, 2, 3}, DuplicateApprovals: []int{4}})
	if err != nil || r.BalanceAfter != -300 || r.ImportedCount != 3 {
		t.Fatalf("legitimate duplicate %+v %v", r, err)
	}
	changed := importInput(account, "Date,Amount,Description,Category,Reference\n2024-02-01,-9,Changed bank row,Food,A")
	next := importPreview(t, uid, changed)
	if next.Preview.DuplicateCount != 1 || next.Preview.Rows[0].Duplicates[0].MatchType != "reference" {
		t.Fatalf("reference not detected %+v", next.Preview)
	}
	// Manual records with a different description still deserve review on date+amount.
	manualAt := time.Date(2024, 3, 2, 14, 0, 0, 0, time.UTC)
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: -250, Currency: "USD", OccurredAt: manualAt, Note: "Manually entered"}); err != nil {
		t.Fatal(err)
	}
	manual := importPreview(t, uid, importInput(account, "Date,Amount,Description,Category,Reference\n2024-03-02,-2.50,Different wording,Food,NEW"))
	if manual.Preview.DuplicateCount != 1 || manual.Preview.Rows[0].Duplicates[0].MatchType != "date_amount" {
		t.Fatalf("manual match %+v", manual.Preview)
	}
}

func TestImportConcurrentCommitReplayAndStalePreview(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	svc := service.NewImportService(pool, q, sumSvc)
	in := importInput(account, "Date,Amount,Description,Category,Reference\n2024-01-01,-1,Purchase,Food,A")
	v := importPreview(t, uid, in)
	selection := importReady(v)
	var wg sync.WaitGroup
	receipts := make([]service.ImportReceipt, 8)
	errs := make([]error, 8)
	for i := range receipts {
		wg.Add(1)
		go func() { defer wg.Done(); receipts[i], errs[i] = svc.Commit(ctx, uid, v.ID, selection) }()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil || receipts[i].ImportedCount != 1 || receipts[i].TransactionIDs[0] != receipts[0].TransactionIDs[0] {
			t.Fatalf("replay %d: %+v %v", i, receipts[i], err)
		}
	}
	if got := importContext(t, uid, account); got.BalanceMinor != -100 || got.EntryCount != 1 {
		t.Fatalf("duplicate replay %+v", got)
	}
	if _, err := svc.Commit(ctx, uid, v.ID, service.ImportSelection{}); !errors.Is(err, service.ErrImportCommitted) {
		t.Fatal(err)
	}
	// Two independently reviewed batches cannot both write against the same snapshot.
	in.CSV = "Date,Amount,Description,Category,Reference\n2024-02-01,-2,Second,Food,B"
	one, two := importPreview(t, uid, in), importPreview(t, uid, in)
	results := make([]error, 2)
	for i, v := range []service.ImportView{one, two} {
		wg.Add(1)
		go func() { defer wg.Done(); _, results[i] = svc.Commit(ctx, uid, v.ID, importReady(v)) }()
	}
	wg.Wait()
	successes, stale := 0, 0
	for _, err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, service.ErrImportChanged) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("results %+v", results)
	}
	fresh := importPreview(t, uid, importInput(account, "Date,Amount,Description,Category,Reference\n2024-03-01,-3,Third,Food,C"))
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: 10, Currency: "USD", OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Commit(ctx, uid, fresh.ID, importReady(fresh)); !errors.Is(err, service.ErrImportChanged) {
		t.Fatal(err)
	}
	// A lost-response retry still returns the original receipt after later activity.
	replay, err := svc.Commit(ctx, uid, v.ID, selection)
	if err != nil || replay.BalanceAfter != -100 {
		t.Fatalf("durable receipt %+v %v", replay, err)
	}
}

func TestImportStartingBalanceCorrection(t *testing.T) {
	ctx := context.Background()
	uid, _ := newUserWithAccount(t, "USD")
	account, err := service.NewAccountService(pool, q).Create(ctx, service.AccountInput{UserID: uid, Name: "Historical bank", Currency: "USD", OpeningMinor: 9000, OpeningAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	in := importInput(account.ID, "Date,Amount,Description,Category,Reference\n2024-01-02,-10,Groceries,Food,A\n2024-01-03,5,Refund,Food,B")
	keep := importPreview(t, uid, in)
	if keep.Preview.InvalidCount != 2 {
		t.Fatalf("before starting date accepted %+v", keep.Preview)
	}
	in.Starting = service.StartingBalancePlan{Mode: "set", AmountMinor: -2000, Date: "2024-01-01"}
	corrected := importPreview(t, uid, in)
	receipt := importCommit(t, uid, corrected)
	if receipt.BalanceBefore != 9000 || receipt.BalanceAfter != -2500 || receipt.StartingBefore != 9000 || receipt.StartingAfter != -2000 {
		t.Fatalf("correction %+v", receipt)
	}
	state := importContext(t, uid, account.ID)
	if state.OpeningCount != 1 || state.OpeningMinor != -2000 || state.TrackingStartAt.Format("2006-01-02") != "2024-01-01" {
		t.Fatalf("context %+v", state)
	}
	if _, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: state.OpeningID}); err != nil {
		t.Fatal(err)
	}
	state = importContext(t, uid, account.ID)
	if state.OpeningID != 0 || state.TrackingStartAt != nil || state.BalanceMinor != -500 {
		t.Fatalf("reverse starting %+v", state)
	}
}

func TestImportRollsBackEverySelectedRowAndCategory(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	v := importPreview(t, uid, importInput(account, "Date,Amount,Description,Category,Reference\n2024-01-01,-1,First,New category,A\n2024-01-02,-2,Second,Other category,B"))
	sql := fmt.Sprintf(`CREATE FUNCTION test_import_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.user_id = %d AND NEW.row_number = 3 THEN RAISE EXCEPTION 'deliberate import failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_import_failure BEFORE INSERT ON imported_transactions FOR EACH ROW EXECUTE FUNCTION test_import_failure();`, uid)
	if _, err := pool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		if _, err := pool.Exec(ctx, `DROP TRIGGER IF EXISTS test_import_failure ON imported_transactions; DROP FUNCTION IF EXISTS test_import_failure();`); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(cleanup)
	svc := service.NewImportService(pool, q, sumSvc)
	if _, err := svc.Commit(ctx, uid, v.ID, importReady(v)); err == nil {
		t.Fatal("injected failure accepted")
	}
	state := importContext(t, uid, account)
	cats, err := q.ListCategoriesByUser(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	var origins, outbox int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM imported_transactions WHERE user_id=$1), (SELECT count(*) FROM outbox WHERE payload->>'user_id' = $1::bigint::text)`, uid).Scan(&origins, &outbox); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.Get(ctx, uid, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.EntryCount != 0 || len(cats) != 0 || origins != 0 || outbox != 0 || saved.Status != "preview" {
		t.Fatalf("partial write: %+v cats %d origins %d outbox %d stage %s", state, len(cats), origins, outbox, saved.Status)
	}
	cleanup()
	r := importCommit(t, uid, v)
	if r.ImportedCount != 2 || r.BalanceAfter != -300 {
		t.Fatalf("retry %+v", r)
	}
}

func TestImportOwnershipExpiryAndPreviewRetention(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	other, otherAccount := newUserWithAccount(t, "USD")
	svc := service.NewImportService(pool, q, sumSvc)
	in := importInput(account, "Date,Amount,Description,Category,Reference\n2024-01-01,-1,Coffee,Food,A")
	v := importPreview(t, uid, in)
	if _, err := svc.Get(ctx, other, v.ID); !errors.Is(err, service.ErrImportNotFound) {
		t.Fatal(err)
	}
	if _, err := svc.Commit(ctx, other, v.ID, importReady(v)); !errors.Is(err, service.ErrImportNotFound) {
		t.Fatal(err)
	}
	foreign := in
	foreign.AccountID = otherAccount
	if _, err := svc.Preview(ctx, uid, foreign); !errors.Is(err, service.ErrAccountNotFound) {
		t.Fatal(err)
	}
	for _, path := range []string{fmt.Sprintf("/imports/%d", v.ID), fmt.Sprintf("/accounts/%d/import-context", account), fmt.Sprintf("/accounts/%d/balance-checks", account), fmt.Sprintf("/activity?import_id=%d", v.ID)} {
		importRequest(t, other, http.MethodGet, path, "", http.StatusNotFound)
	}
	importRequest(t, 0, http.MethodPost, "/imports/inspect", `{"csv":"Date,Amount\n2024-01-01,1"}`, http.StatusUnauthorized)
	if _, err := pool.Exec(ctx, `UPDATE import_sessions SET expires_at=now()-interval '1 second' WHERE id=$1`, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Commit(ctx, uid, v.ID, importReady(v)); !errors.Is(err, service.ErrImportExpired) {
		t.Fatal(err)
	}
	importRequest(t, uid, http.MethodGet, fmt.Sprintf("/imports/%d", v.ID), "", http.StatusGone)
	var first service.ImportView
	for i := 0; i < 12; i++ {
		next := importPreview(t, uid, in)
		if i == 0 {
			first = next
		}
	}
	if _, err := svc.Get(ctx, uid, first.ID); !errors.Is(err, service.ErrImportNotFound) {
		t.Fatal("old preview not pruned", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM import_sessions WHERE user_id=$1 AND status='preview'`, uid).Scan(&count); err != nil || count != 10 {
		t.Fatalf("pending %d %v", count, err)
	}
}

func TestImportBackdatedBalanceCacheAndRefund(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: 1000, Currency: "USD", OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	day := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	if err := q.UpsertBalanceSnapshot(ctx, sqlc.UpsertBalanceSnapshotParams{AccountID: account, AsOfDate: day, BalanceMinor: 0}); err != nil {
		t.Fatal(err)
	}
	month := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	primed, err := sumSvc.GetMonthlySummary(ctx, account, month)
	if err != nil || primed.Net != 0 {
		t.Fatal(primed, err)
	}
	in := importInput(account, "Date,Amount,Description,Category,Reference,Type\n2024-01-02,-1,Clothing,Clothing,A,expense\n2024-01-03,0.40,Partial return,Clothing,B,refund")
	in.Mapping.Type = importColumn(5)
	v := importPreview(t, uid, in)
	importCommit(t, uid, v)
	balance, err := balSvc.CurrentBalance(ctx, account, time.Now())
	if err != nil || balance.Balance != 940 {
		t.Fatalf("snapshot lost imported records: %+v %v", balance, err)
	}
	summary, err := sumSvc.GetMonthlySummary(ctx, account, month)
	if err != nil || summary.Net != -60 {
		t.Fatalf("stale summary cache: %+v %v", summary, err)
	}
	var spending map[string]any
	getWorkspace(t, importsRouter(uid), "/spending?type=all&currency=USD", &spending)
	if spending["income_minor"] != float64(1000) || spending["spending_minor"] != float64(60) {
		t.Fatalf("refund imported as income: %+v", spending)
	}
}

func TestImportMigrationDownPreservesPopulatedProvenance(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	v := importPreview(t, uid, importInput(account, "Date,Amount,Description,Category,Reference\n2024-01-01,-1,Coffee,Food,A"))
	importCommit(t, uid, v)
	down, err := os.ReadFile("../../migrations/000019_imports_and_balance_checks.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, string(down)); err == nil || !strings.Contains(err.Error(), "cannot remove import provenance") {
		t.Fatalf("populated down migration: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	// Exercise the empty down path transactionally. Rollback restores the test
	// suite's tables and records; this never targets the development database.
	empty, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = empty.Rollback(ctx) }()
	planDown, err := os.ReadFile("../../migrations/000022_history_import_and_plans.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := empty.Exec(ctx, "DELETE FROM planned_transactions; UPDATE transactions SET exclude_from_reports=false WHERE exclude_from_reports"); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.Exec(ctx, string(planDown)); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.Exec(ctx, "DELETE FROM imported_transactions; DELETE FROM balance_checks;"); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.Exec(ctx, string(down)); err != nil {
		t.Fatalf("empty down migration: %v", err)
	}
	if err := empty.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got := importContext(t, uid, account); got.BalanceMinor != -100 {
		t.Fatalf("rollback lost ledger: %+v", got)
	}
	saved, err := service.NewImportService(pool, q, sumSvc).Get(ctx, uid, v.ID)
	if err != nil || saved.Receipt == nil || saved.Receipt.ImportedCount != 1 {
		t.Fatalf("rollback lost receipt: %+v %v", saved, err)
	}
}

func TestBalanceChecksSnapshotsIdempotencyAndNoAdjustment(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	other, _ := newUserWithAccount(t, "USD")
	svc := service.NewBalanceCheckService(pool, q)
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: -1200, Currency: "USD", OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	before := importContext(t, uid, account)
	in := service.BalanceCheckInput{UserID: uid, AccountID: account, ActualMinor: -1000, Note: "Bank posted balance", Key: "first-check"}
	results := make([]sqlc.BalanceCheck, 8)
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() { defer wg.Done(); results[i], errs[i] = svc.Check(ctx, in) }()
	}
	wg.Wait()
	for i, result := range results {
		if errs[i] != nil || result.ID != results[0].ID || result.RecordedMinor != -1200 || result.DifferenceMinor != 200 || result.ActualMinor != -1000 {
			t.Fatalf("check %+v %v", result, errs[i])
		}
	}
	after := importContext(t, uid, account)
	if before != after {
		t.Fatalf("comparison adjusted ledger: %+v %+v", before, after)
	}
	mismatch := in
	mismatch.ActualMinor = 0
	if _, err := svc.Check(ctx, mismatch); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	for _, actual := range []int64{0, -1200, -1300} {
		input := in
		input.Key = ""
		input.ActualMinor = actual
		c, err := svc.Check(ctx, input)
		if err != nil || c.DifferenceMinor != actual+1200 {
			t.Fatalf("actual %d: %+v %v", actual, c, err)
		}
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: 100, Currency: "USD", OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	replay, err := svc.Check(ctx, in)
	if err != nil || replay.LedgerRevision != before.Revision || replay.RecordedMinor != -1200 {
		t.Fatalf("retry changed snapshot %+v %v", replay, err)
	}
	history, current, err := svc.History(ctx, uid, account)
	if err != nil || len(history) != 4 || current.Revision == replay.LedgerRevision || current.BalanceMinor != -1100 {
		t.Fatalf("stale check history %+v %+v %v", history, current, err)
	}
	foreign := in
	foreign.UserID = other
	foreign.Key = ""
	if _, err := svc.Check(ctx, foreign); !errors.Is(err, service.ErrAccountNotFound) {
		t.Fatal(err)
	}
	excessive := in
	excessive.Key = ""
	excessive.ActualMinor = money.MaxSafeMinor + 1
	if _, err := svc.Check(ctx, excessive); !errors.Is(err, service.ErrMoneyRange) {
		t.Fatal(err)
	}
	w := importRequest(t, uid, http.MethodPost, fmt.Sprintf("/accounts/%d/balance-checks", account), `{"actual_minor":0}`, http.StatusCreated)
	if strings.Contains(w.Body.String(), "request_hash") || strings.Contains(w.Body.String(), "request_key") {
		t.Fatal("internal retry identity exposed")
	}
	importRequest(t, uid, http.MethodPost, fmt.Sprintf("/accounts/%d/balance-checks", account), `{}`, http.StatusUnprocessableEntity)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["difference_minor"] != float64(1100) {
		t.Fatal(body, err)
	}
}
