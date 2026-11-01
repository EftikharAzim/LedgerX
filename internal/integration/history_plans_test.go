package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
)

func TestFullHistoryAtomicImportExclusionsAndDuplicateReview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	uid, account := newUserWithAccount(t, "BDT")
	// Keep this large fixture's event backlog isolated from publisher tests.
	// The test below verifies every event before this scoped cleanup.
	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(), "DELETE FROM outbox WHERE (payload->>'user_id')::bigint=$1", uid)
		if err != nil {
			t.Error(err)
		}
	})
	var csv strings.Builder
	csv.WriteString("ID;Note;Amount;Category;Account;Currency;Date;Event;Exclude Report\n")
	for i := 0; i < 6753; i++ {
		wallet, currency := "Daily", "BDT"
		if i == 6752 {
			wallet, currency = "Travel", "USD"
		}
		fmt.Fprintf(&csv, "ref-%d;Synthetic %d;-1.25;Food;%s;%s;23/05/%d;;%t\n", i, i, wallet, currency, 2020+i%7, i < 43)
	}
	doc, err := importing.Inspect(csv.String(), "auto")
	if err != nil {
		t.Fatal(err)
	}
	in := importInput(account, csv.String())
	in.SourceGroup = doc.SourceGroups[0].Key
	in.Mapping = importing.Mapping{Date: importColumn(6), Amount: importColumn(2), Note: importColumn(1), Category: importColumn(3), Currency: importColumn(5), Reference: importColumn(0), ExcludeReport: importColumn(8), AmountMode: "signed", DateFormat: "day_first", NumberStyle: "dot", SignConvention: "income_positive"}
	svc := service.NewImportService(pool, q, sumSvc)
	v, err := svc.Preview(ctx, uid, in)
	if err != nil {
		t.Fatal(err)
	}
	if v.Preview.ReadyCount != 6752 || v.Preview.OtherSourceCount != 1 {
		t.Fatal("full source history not ready")
	}
	started := time.Now()
	r, err := svc.Commit(ctx, uid, v.ID, importReady(v))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("6,752 posted records committed atomically in %s", time.Since(started))
	if r.ImportedCount != 6752 || len(r.TransactionIDs) != 6752 || r.ExcludedReportCount != 43 || r.BalanceAfter != -6752*125 {
		t.Fatal("incorrect history receipt")
	}
	var unbalanced, events, origins int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT t.id FROM transactions t JOIN postings p ON p.transaction_id=t.id WHERE t.user_id=$1 GROUP BY t.id HAVING sum(p.amount_minor)<>0 OR count(*)<>2) x`, uid).Scan(&unbalanced); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM outbox WHERE (payload->>'user_id')::bigint=$1),(SELECT count(*) FROM imported_transactions WHERE user_id=$1)`, uid).Scan(&events, &origins); err != nil {
		t.Fatal(err)
	}
	if unbalanced != 0 || events != 6752 || origins != 6752 {
		t.Fatal("bulk write missed ledger invariants", unbalanced, events, origins)
	}
	var spending struct {
		Spending int64 `json:"spending_minor"`
	}
	getWorkspace(t, importsRouter(uid), "/spending?type=all&currency=BDT", &spending)
	if spending.Spending != (6752-43)*125 {
		t.Fatal("report exclusions changed balance semantics", spending)
	}
	again, err := svc.Preview(ctx, uid, in)
	if err != nil {
		t.Fatal(err)
	}
	if again.Preview.DuplicateCount != 6752 || again.Preview.ReadyCount != 0 {
		t.Fatal("reimport lost references")
	}
	// A failed COPY must roll back headers, postings, categories, origins and events.
	_, err = pool.Exec(ctx, `CREATE FUNCTION reject_synthetic_copy() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.note='Synthetic rollback last' THEN RAISE EXCEPTION 'synthetic failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_synthetic_copy BEFORE INSERT ON transactions FOR EACH ROW EXECUTE FUNCTION reject_synthetic_copy()`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_synthetic_copy ON transactions; DROP FUNCTION IF EXISTS reject_synthetic_copy()`)
	}()
	broken := importInput(account, "Date,Amount,Note,Category,Reference\n2024-01-01,-1,Synthetic rollback first,Rollback Category,rollback-a\n2024-01-02,-2,Synthetic rollback last,Rollback Category,rollback-b")
	bv := importPreview(t, uid, broken)
	if _, err = svc.Commit(ctx, uid, bv.ID, importReady(bv)); err == nil {
		t.Fatal("injected COPY failure accepted")
	}
	if got := importContext(t, uid, account); got.BalanceMinor != r.BalanceAfter || got.EntryCount != 6752 {
		t.Fatal("partial history persisted")
	}
	cats, err := q.ListCategoriesByUser(ctx, uid)
	if err != nil || len(cats) != 1 {
		t.Fatal("partial category persisted", err)
	}
}

func TestPlansStayOutsideLedgerConfirmOnceAndRestore(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	future := time.Now().UTC().AddDate(0, 2, 0).Format("2006-01-02")
	in := importInput(account, fmt.Sprintf("Date,Amount,Note,Category,Reference,Exclude Report\n2024-01-01,-1,Past,Food,past,false\n%s,-12,Future one,Bills,future-a,true\n%s,-5,Future two,Bills,future-b,false", future, future))
	in.Mapping.ExcludeReport = importColumn(5)
	v := importPreview(t, uid, in)
	if v.Preview.PlannedCount != 2 || v.Preview.ProjectedMinor != -100 {
		t.Fatal("plans changed projected balance")
	}
	receipt := importCommit(t, uid, v)
	if receipt.PlannedCount != 2 || len(receipt.TransactionIDs) != 1 || receipt.BalanceAfter != -100 {
		t.Fatal("plans posted too soon")
	}
	if again := importPreview(t, uid, in); again.Preview.DuplicateCount != 3 {
		t.Fatal("pending plans not detected on reimport")
	}
	plans := service.NewPlanService(pool, q, sumSvc)
	foreign, _ := newUserWithAccount(t, "USD")
	if _, err := plans.Resolve(ctx, foreign, receipt.PlannedIDs[0], "2024-01-05", false); !errors.Is(err, service.ErrPlanNotFound) {
		t.Fatal("plan ownership bypass", err)
	}
	var wg sync.WaitGroup
	ids := make([]int64, 6)
	errs := make([]error, 6)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], errs[i] = plans.Resolve(ctx, uid, receipt.PlannedIDs[0], "2024-01-05", false)
		}()
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] == 0 || ids[i] != ids[0] {
			t.Fatal("plan retry posted twice", errs[i])
		}
	}
	if got := importContext(t, uid, account); got.BalanceMinor != -1300 || got.EntryCount != 2 {
		t.Fatal("confirmation balance incorrect", got)
	}
	if again := importPreview(t, uid, in); again.Preview.DuplicateCount != 3 {
		t.Fatal("posted plan lost source reference")
	}
	manual := service.PlanInput{AccountID: account, AmountMinor: -333, Currency: "USD", OccurredAt: time.Now().UTC().AddDate(0, 1, 0), Note: "Manual plan"}
	one, err := plans.Create(ctx, uid, "same-manual-request", manual)
	if err != nil {
		t.Fatal(err)
	}
	two, err := plans.Create(ctx, uid, "same-manual-request", manual)
	if err != nil || one.ID != two.ID {
		t.Fatal("manual plan not idempotent", err)
	}
	manual.AmountMinor--
	if _, err := plans.Create(ctx, uid, "same-manual-request", manual); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatal("changed plan request reused", err)
	}
	// Pending and already-confirmed plans, report flags and import receipts survive ID remapping.
	backupSvc := service.NewWorkspaceService(pool, q)
	backup, err := backupSvc.Backup(ctx, uid, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(backup)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "creation_key") || strings.Contains(string(raw), "creation_hash") {
		t.Fatal("backup leaked retry credentials")
	}
	target := emptyWorkspaceUser(t)
	preview, err := backupSvc.Preview(ctx, target, "plans.ledgerx.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.PlanCount != 3 {
		t.Fatal("backup dropped plans")
	}
	if _, err := backupSvc.Restore(ctx, target, preview.ID, preview.Checksum); err != nil {
		t.Fatal(err)
	}
	restored, err := backupSvc.Backup(ctx, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Data.Plans) != 3 {
		t.Fatal("restored plans missing")
	}
	for _, p := range restored.Data.Plans {
		if p.Status == "posted" {
			if !p.ExcludeReport || p.TransactionID == ids[0] {
				t.Fatal("restored plan link/flag not remapped")
			}
		}
	}
	if _, err := plans.Resolve(ctx, uid, receipt.PlannedIDs[1], "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.Resolve(ctx, uid, receipt.PlannedIDs[1], "", true); err != nil {
		t.Fatal("cancel retry", err)
	}
	if got := importContext(t, uid, account); got.BalanceMinor != -1300 {
		t.Fatal("cancel changed money")
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", AmountMinor: -1, OccurredAt: time.Now().UTC().AddDate(0, 1, 0)}); !errors.Is(err, service.ErrFutureTransaction) {
		t.Fatal("future direct posting accepted", err)
	}
	exports, err := q.ListPostingsForExport(ctx, sqlc.ListPostingsForExportParams{UserID: uid})
	if err != nil {
		t.Fatal(err)
	}
	flags := 0
	for _, e := range exports {
		if e.ExcludeFromReports {
			flags++
		}
	}
	if flags != 1 {
		t.Fatal("export lost report flag", flags)
	}
	correction := correctRecord(t, uid, ids[0], "edit-excluded-plan", func(d *service.CorrectionDraft) { d.Note = "Edited excluded plan"; d.ExcludeReport = false })
	changed, err := q.GetTransactionForUser(ctx, sqlc.GetTransactionForUserParams{ID: correction.ReplacementID, UserID: uid})
	if err != nil || !changed.ExcludeFromReports {
		t.Fatal("editing lost report exclusion", err)
	}
	if _, err := backupSvc.Backup(ctx, uid, nil); err != nil {
		t.Fatal("edited plans no longer back up", err)
	}
}
