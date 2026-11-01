package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/EftikharAzim/ledgerx/internal/workspace"
)

func emptyWorkspaceUser(t *testing.T) int64 {
	t.Helper()
	userSeq++
	u, err := q.CreateUser(context.Background(), sqlc.CreateUserParams{Email: fmt.Sprintf("recovery-%d-%d@example.com", time.Now().UnixNano(), userSeq), PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateExternalAccount(context.Background(), sqlc.CreateExternalAccountParams{UserID: u.ID, Currency: "USD"}); err != nil {
		t.Fatal(err)
	}
	return u.ID
}
func correctionInput(t *testing.T, uid, id int64, change func(*service.CorrectionDraft)) service.CorrectionInput {
	t.Helper()
	svc := service.NewCorrectionService(pool, q, sumSvc)
	c, err := svc.Context(context.Background(), uid, id)
	if err != nil {
		t.Fatal(err)
	}
	d := c.Draft
	change(&d)
	in := service.CorrectionInput{Draft: d, SourceToken: c.SourceToken}
	plan, err := svc.Preview(context.Background(), uid, id, in)
	if err != nil {
		t.Fatal(err)
	}
	in.PreviewToken = plan.PreviewToken
	return in
}
func correctRecord(t *testing.T, uid, id int64, key string, change func(*service.CorrectionDraft)) service.CorrectionReceipt {
	t.Helper()
	in := correctionInput(t, uid, id, change)
	r, err := service.NewCorrectionService(pool, q, sumSvc).Correct(context.Background(), uid, id, key, in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCorrectionAtomicEditAcrossAccountsAndRetries(t *testing.T) {
	ctx := context.Background()
	uid, source := newUserWithAccount(t, "USD")
	target, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "Target", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	original, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: source, AmountMinor: -1000, Currency: "USD", OccurredAt: time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC), Note: "Wrong purchase"})
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewCorrectionService(pool, q, sumSvc)
	in := correctionInput(t, uid, original.ID, func(d *service.CorrectionDraft) {
		d.AccountID = target.ID
		d.AmountMinor = -1200
		d.Note = "Correct purchase"
		d.OccurredAt = d.OccurredAt.AddDate(0, 1, 0)
	})
	if state := importContext(t, uid, source); state.BalanceMinor != -1000 || state.EntryCount != 1 {
		t.Fatal("preview wrote money", state)
	}
	receipts := make([]service.CorrectionReceipt, 8)
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range receipts {
		wg.Add(1)
		go func() { defer wg.Done(); receipts[i], errs[i] = svc.Correct(ctx, uid, original.ID, "retry-edit", in) }()
	}
	wg.Wait()
	for i, r := range receipts {
		if errs[i] != nil || r.ReversalID != receipts[0].ReversalID || r.ReplacementID != receipts[0].ReplacementID || r.OriginalID != original.ID {
			t.Fatal(r, errs[i])
		}
	}
	if state := importContext(t, uid, source); state.BalanceMinor != 0 || state.EntryCount != 2 {
		t.Fatal(state)
	}
	if state := importContext(t, uid, target.ID); state.BalanceMinor != -1200 || state.EntryCount != 1 {
		t.Fatal(state)
	}
	modified := in
	modified.Draft.Note = "Changed retry"
	if _, err := svc.Correct(ctx, uid, original.ID, "retry-edit", modified); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	if _, err := svc.Context(ctx, uid, original.ID); !errors.Is(err, service.ErrAlreadyReversed) {
		t.Fatal(err)
	}
}

func TestCorrectionStaleOwnershipTransferAndStartingMoney(t *testing.T) {
	ctx := context.Background()
	uid, source := newUserWithAccount(t, "USD")
	other, foreign := newUserWithAccount(t, "USD")
	svc := service.NewCorrectionService(pool, q, sumSvc)
	original, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: source, AmountMinor: 1000, Currency: "USD", OccurredAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Context(ctx, other, original.ID); !errors.Is(err, service.ErrTxNotFound) {
		t.Fatal(err)
	}
	context, err := svc.Context(ctx, uid, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Preview(ctx, uid, original.ID, service.CorrectionInput{Draft: context.Draft, SourceToken: context.SourceToken}); !errors.Is(err, service.ErrCorrectionUnchanged) {
		t.Fatal(err)
	}
	in := correctionInput(t, uid, original.ID, func(d *service.CorrectionDraft) { d.AmountMinor = 1500 })
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: source, AmountMinor: 1, Currency: "USD", OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Correct(ctx, uid, original.ID, "stale", in); !errors.Is(err, service.ErrCorrectionChanged) {
		t.Fatal(err)
	}
	context, err = svc.Context(ctx, uid, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	bad := context.Draft
	bad.AccountID = foreign
	if _, err := svc.Preview(ctx, uid, original.ID, service.CorrectionInput{Draft: bad, SourceToken: context.SourceToken}); !errors.Is(err, service.ErrAccountNotFound) {
		t.Fatal(err)
	}
	target, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "Savings", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := txSvc.Transfer(ctx, service.TransferInput{UserID: uid, FromAccountID: source, ToAccountID: target.ID, AmountMinor: 200, Currency: "USD", OccurredAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	receipt := correctRecord(t, uid, transfer.ID, "transfer-edit", func(d *service.CorrectionDraft) { d.AmountMinor = -300; d.Note = "Correct transfer" })
	if receipt.New.ToAccountID != target.ID || importContext(t, uid, target.ID).BalanceMinor != 300 {
		t.Fatal(receipt)
	}
	opening, err := service.NewAccountService(pool, q).Create(ctx, service.AccountInput{UserID: uid, Name: "Starting correction", Currency: "USD", OpeningMinor: 5000, OpeningAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	openingContext := importContext(t, uid, opening.ID)
	zero := correctRecord(t, uid, openingContext.OpeningID, "zero-start", func(d *service.CorrectionDraft) { d.AmountMinor = 0; d.DateOnly = true })
	state := importContext(t, uid, opening.ID)
	if zero.ReplacementID != 0 || state.BalanceMinor != 0 || state.OpeningCount != 0 || state.TrackingStartAt == nil {
		t.Fatal(zero, state)
	}
}

func financialBackupFixture(t *testing.T) (int64, workspace.Backup) {
	t.Helper()
	ctx := context.Background()
	uid, target := newUserWithAccount(t, "USD")
	bank, err := service.NewAccountService(pool, q).Create(ctx, service.AccountInput{UserID: uid, Name: "Bank", Currency: "USD", OpeningMinor: 5000, OpeningAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	food, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: uid, Name: "Food"})
	if err != nil {
		t.Fatal(err)
	}
	purchase, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: bank.ID, AmountMinor: -1000, Currency: "USD", OccurredAt: time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC), Note: "Purchase", CategoryID: food.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txSvc.Transfer(ctx, service.TransferInput{UserID: uid, FromAccountID: bank.ID, ToAccountID: target, AmountMinor: 200, Currency: "USD", OccurredAt: time.Date(2024, 1, 3, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: bank.ID, AmountMinor: 200, Currency: "USD", OccurredAt: time.Date(2024, 1, 4, 12, 0, 0, 0, time.UTC), Note: "Partial refund", EntryType: "refund", CategoryID: food.ID}); err != nil {
		t.Fatal(err)
	}
	imported := importPreview(t, uid, importInput(bank.ID, "Date,Amount,Description,Category,Reference\n2024-01-05,-1.23,Imported coffee,Food,A"))
	r := importCommit(t, uid, imported)
	checks := service.NewBalanceCheckService(pool, q)
	state := importContext(t, uid, bank.ID)
	if _, err := checks.Check(ctx, service.BalanceCheckInput{UserID: uid, AccountID: bank.ID, ActualMinor: state.BalanceMinor, Key: "before-edit"}); err != nil {
		t.Fatal(err)
	}
	correctRecord(t, uid, purchase.ID, "move-purchase", func(d *service.CorrectionDraft) {
		d.AccountID = target
		d.AmountMinor = -1200
		d.Note = "Corrected purchase"
	})
	one := correctRecord(t, uid, r.TransactionIDs[0], "import-edit-one", func(d *service.CorrectionDraft) { d.Note = "Coffee receipt" })
	correctRecord(t, uid, one.ReplacementID, "import-edit-two", func(d *service.CorrectionDraft) { d.AmountMinor = -150 })
	if _, err := service.NewAccountService(pool, q).Create(ctx, service.AccountInput{UserID: uid, Name: "Yen", Currency: "JPY", OpeningMinor: -500, OpeningAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	state = importContext(t, uid, bank.ID)
	if _, err := checks.Check(ctx, service.BalanceCheckInput{UserID: uid, AccountID: bank.ID, ActualMinor: state.BalanceMinor, Key: "after-edit"}); err != nil {
		t.Fatal(err)
	}
	b, err := service.NewWorkspaceService(pool, q).Backup(ctx, uid, map[int64]workspace.Appearance{bank.ID: {Icon: "card", Color: "lavender"}})
	if err != nil {
		t.Fatal(err)
	}
	return uid, b
}

func TestWorkspaceBackupRestoreFinancialRoundTrip(t *testing.T) {
	ctx := context.Background()
	source, b := financialBackupFixture(t)
	svc := service.NewWorkspaceService(pool, q)
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"password_hash", "refresh_token", "request_key", "request_hash", "file_path", "user_id"} {
		if strings.Contains(string(raw), `"`+secret+`"`) {
			t.Fatalf("backup contains private server field %s", secret)
		}
	}
	target := emptyWorkspaceUser(t)
	v, err := svc.Preview(ctx, target, "my.ledgerx.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if v.Summary.ImportCount != 1 || v.Summary.CorrectionCount != 3 || v.Summary.CheckCount != 2 || len(v.Summary.Accounts) != 3 {
		t.Fatal(v.Summary)
	}
	before, err := q.WorkspaceIsEmpty(ctx, target)
	if err != nil || !before.Bool {
		t.Fatal("preview changed workspace", before, err)
	}
	receipts := make([]service.RestoreReceipt, 8)
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range receipts {
		wg.Add(1)
		go func() { defer wg.Done(); receipts[i], errs[i] = svc.Restore(ctx, target, v.ID, v.Checksum) }()
	}
	wg.Wait()
	receipt := receipts[0]
	for i, r := range receipts {
		if errs[i] != nil || r.CompletedAt != receipt.CompletedAt || len(r.Accounts) != len(receipt.Accounts) {
			t.Fatal("concurrent restore replay", r, errs[i])
		}
	}
	again, err := svc.Restore(ctx, target, v.ID, v.Checksum)
	if err != nil || again.CompletedAt != receipt.CompletedAt || len(again.Accounts) != len(receipt.Accounts) {
		t.Fatal(again, err)
	}
	sourceByName := map[string]workspace.AccountSummary{}
	sourceSummary, err := b.Validate()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range sourceSummary.Accounts {
		sourceByName[a.Name] = a
	}
	restored, err := svc.Backup(ctx, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	restoredSummary, err := restored.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if restoredSummary.TransactionCount != sourceSummary.TransactionCount || restoredSummary.PostingCount != sourceSummary.PostingCount || restoredSummary.ImportCount != 1 || restoredSummary.CorrectionCount != 3 || restoredSummary.CheckCount != 2 {
		t.Fatal(restoredSummary)
	}
	for _, a := range restoredSummary.Accounts {
		if want := sourceByName[a.Name]; a.BalanceMinor != want.BalanceMinor || a.Currency != want.Currency || a.SourceID == want.SourceID {
			t.Fatalf("restored account differs: %+v %+v", a, want)
		}
	}
	for _, a := range receipt.Accounts {
		if a.Name == "Bank" {
			if a.Appearance == nil || a.Appearance.Icon != "card" || a.Appearance.Color != "lavender" {
				t.Fatal("appearance lost", a)
			}
			history, current, err := service.NewBalanceCheckService(pool, q).History(ctx, target, a.ID)
			if err != nil || len(history) != 2 || history[0].LedgerRevision != current.Revision || history[1].LedgerRevision == current.Revision {
				t.Fatal("revision mapping lost", history, current, err)
			}
		}
	}
	saved, err := svc.Get(ctx, target, v.ID)
	if err != nil || saved.Receipt == nil {
		t.Fatal(saved, err)
	}
	if _, err := svc.Get(ctx, source, v.ID); !errors.Is(err, service.ErrRestoreNotFound) {
		t.Fatal("foreign restore visible", err)
	}
	second, err := svc.Preview(ctx, target, "repeat.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Restore(ctx, target, second.ID, second.Checksum); !errors.Is(err, service.ErrWorkspaceNotEmpty) {
		t.Fatal("restore overwrote current data", err)
	}
}

func TestRestoreReplacesEmptySystemSnapshotsAndRacingReviews(t *testing.T) {
	ctx := context.Background()
	target := emptyWorkspaceUser(t)
	external, err := q.GetExternalAccount(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	day := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	if err := q.UpsertBalanceSnapshot(ctx, sqlc.UpsertBalanceSnapshotParams{AccountID: external.ID, AsOfDate: day, BalanceMinor: 0}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	backup := workspace.Backup{Format: "ledgerx.workspace", Version: 1, ExportedAt: time.Now().UTC(), Data: workspace.Data{Accounts: []workspace.Account{{ID: 1, Name: "Bank", Currency: "USD", Active: true, Kind: "normal", CreatedAt: at}, {ID: 2, Name: "External", Currency: "USD", Active: true, Kind: "external", CreatedAt: at}}, Transactions: []workspace.Transaction{{ID: 1, Currency: "USD", OccurredAt: at, CreatedAt: at, EntryType: "standard"}}, Postings: []workspace.Posting{{ID: 1, TransactionID: 1, AccountID: 1, AmountMinor: 100, CreatedAt: at}, {ID: 2, TransactionID: 1, AccountID: 2, AmountMinor: -100, CreatedAt: at}}}}
	if err := backup.Seal(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(backup)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewWorkspaceService(pool, q)
	first, err := svc.Preview(ctx, target, "first.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Preview(ctx, target, "second.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	views := []service.RestoreView{first, second}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, v := range views {
		wg.Add(1)
		go func() { defer wg.Done(); _, errs[i] = svc.Restore(ctx, target, v.ID, v.Checksum) }()
	}
	wg.Wait()
	successes, conflicts := 0, 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, service.ErrWorkspaceNotEmpty) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal(errs)
	}
	currentExternal, err := q.GetExternalAccount(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if currentExternal.ID == external.ID {
		t.Fatal("old snapshot account reused")
	}
	balance, err := balSvc.CurrentBalance(ctx, currentExternal.ID, time.Now())
	if err != nil || balance.Balance != -100 {
		t.Fatalf("restored insertion history hidden behind snapshot: %+v %v", balance, err)
	}
}

func TestCorrectionFailureRollsBackAndCategoryChangeInvalidatesReview(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	svc := service.NewCorrectionService(pool, q, sumSvc)
	original, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: -100, Currency: "USD", OccurredAt: time.Now().UTC(), Note: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	in := correctionInput(t, uid, original.ID, func(d *service.CorrectionDraft) { d.AmountMinor = -200 })
	trigger := fmt.Sprintf(`CREATE FUNCTION test_correction_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.user_id=%d THEN RAISE EXCEPTION 'injected correction failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_correction_failure BEFORE INSERT ON transaction_corrections FOR EACH ROW EXECUTE FUNCTION test_correction_failure();`, uid)
	if _, err := pool.Exec(ctx, trigger); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		if _, err := pool.Exec(ctx, `DROP TRIGGER IF EXISTS test_correction_failure ON transaction_corrections; DROP FUNCTION IF EXISTS test_correction_failure();`); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(cleanup)
	if _, err := svc.Correct(ctx, uid, original.ID, "failure", in); err == nil {
		t.Fatal("injected correction succeeded")
	}
	state := importContext(t, uid, account)
	if state.BalanceMinor != -100 || state.EntryCount != 1 {
		t.Fatal("partial edit", state)
	}
	if _, err := svc.Context(ctx, uid, original.ID); err != nil {
		t.Fatal("original canceled on failed edit", err)
	}
	cleanup()
	cat, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: uid, Name: "Food"})
	if err != nil {
		t.Fatal(err)
	}
	if err := txSvc.ConfirmCategory(ctx, uid, original.ID, cat.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Correct(ctx, uid, original.ID, "after-category-change", in); !errors.Is(err, service.ErrCorrectionChanged) {
		t.Fatal("category race not detected", err)
	}
	fresh := correctionInput(t, uid, original.ID, func(d *service.CorrectionDraft) { d.AmountMinor = -200; d.CategoryID = 0 })
	receipt, err := svc.Correct(ctx, uid, original.ID, "after-reload", fresh)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := svc.Result(ctx, uid, original.ID, "after-reload")
	if err != nil || recovered.ReplacementID != receipt.ReplacementID {
		t.Fatal(recovered, err)
	}
	if err := txSvc.ConfirmCategory(ctx, uid, original.ID, cat.ID); !errors.Is(err, service.ErrAlreadyReversed) {
		t.Fatal("canceled history remained editable", err)
	}
}

func TestWorkspaceRestoreFailureRollsBackAndCanRetry(t *testing.T) {
	ctx := context.Background()
	_, b := financialBackupFixture(t)
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	target := emptyWorkspaceUser(t)
	svc := service.NewWorkspaceService(pool, q)
	view, err := svc.Preview(ctx, target, "restore.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`CREATE FUNCTION test_restore_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.user_id=%d THEN RAISE EXCEPTION 'injected restore failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_restore_failure BEFORE INSERT ON transaction_corrections FOR EACH ROW EXECUTE FUNCTION test_restore_failure();`, target)
	if _, err := pool.Exec(ctx, trigger); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		if _, err := pool.Exec(ctx, `DROP TRIGGER IF EXISTS test_restore_failure ON transaction_corrections; DROP FUNCTION IF EXISTS test_restore_failure();`); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(cleanup)
	if _, err := svc.Restore(ctx, target, view.ID, view.Checksum); err == nil {
		t.Fatal("injected failure succeeded")
	}
	empty, err := q.WorkspaceIsEmpty(ctx, target)
	if err != nil || !empty.Bool {
		t.Fatal("partial restore", empty, err)
	}
	state, err := svc.Get(ctx, target, view.ID)
	if err != nil || state.Status != "preview" || state.Receipt != nil {
		t.Fatal(state, err)
	}
	cleanup()
	receipt, err := svc.Restore(ctx, target, view.ID, view.Checksum)
	if err != nil || receipt.ID != view.ID {
		t.Fatal(receipt, err)
	}
}

func TestWorkspaceBulkRestorePreservesHistoricalPostings(t *testing.T) {
	ctx := context.Background()
	target := emptyWorkspaceUser(t)
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	b := workspace.Backup{Format: "ledgerx.workspace", Version: 1, ExportedAt: time.Now().UTC(), Data: workspace.Data{Accounts: []workspace.Account{{ID: 1, Name: "Bulk bank", Currency: "USD", Active: true, Kind: "normal", CreatedAt: at}, {ID: 2, Name: "External", Currency: "USD", Active: true, Kind: "external", CreatedAt: at}}}}
	for i := int64(1); i <= 1000; i++ {
		amount := int64(-29)
		if i%2 == 0 {
			amount = 37
		}
		b.Data.Transactions = append(b.Data.Transactions, workspace.Transaction{ID: i, Currency: "USD", OccurredAt: at.Add(time.Duration(i) * time.Hour), CreatedAt: at, EntryType: "standard"})
		b.Data.Postings = append(b.Data.Postings, workspace.Posting{ID: 2*i - 1, TransactionID: i, AccountID: 1, AmountMinor: amount, CreatedAt: at}, workspace.Posting{ID: 2 * i, TransactionID: i, AccountID: 2, AmountMinor: -amount, CreatedAt: at})
	}
	if err := b.Seal(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewWorkspaceService(pool, q)
	view, err := svc.Preview(ctx, target, "bulk.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := svc.Restore(ctx, target, view.ID, view.Checksum)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Summary.TransactionCount != 1000 || receipt.Summary.PostingCount != 2000 || len(receipt.Accounts) != 1 {
		t.Fatal(receipt.Summary)
	}
	balance, err := balSvc.CurrentBalance(ctx, receipt.Accounts[0].ID, time.Now())
	if err != nil || balance.Balance != 4000 {
		t.Fatal(balance, err)
	}
	restored, err := svc.Backup(ctx, target, nil)
	if err != nil || len(restored.Data.Postings) != 2000 {
		t.Fatal("bulk round trip", len(restored.Data.Postings), err)
	}
	for _, p := range restored.Data.Postings {
		if !p.CreatedAt.Equal(at) {
			t.Fatal("historical creation date changed", p)
		}
	}
}

func TestImportedCorrectionMovedAcrossAccountsStillFlagsReimport(t *testing.T) {
	for _, reference := range []string{"BANK-A", ""} {
		t.Run("reference-"+reference, func(t *testing.T) {
			ctx := context.Background()
			uid, bank := newUserWithAccount(t, "USD")
			cash, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "Cash after correction", Currency: "USD"})
			if err != nil {
				t.Fatal(err)
			}
			input := importInput(bank, "Date,Amount,Description,Category,Reference\n2024-01-02,-1,Imported purchase,Food,"+reference)
			original := importPreview(t, uid, input)
			receipt := importCommit(t, uid, original)
			corrected := correctRecord(t, uid, receipt.TransactionIDs[0], "move-import", func(d *service.CorrectionDraft) {
				d.AccountID = cash.ID
				d.AmountMinor = -200
				d.OccurredAt = d.OccurredAt.AddDate(0, 0, 1)
			})
			repeated := importPreview(t, uid, input)
			if repeated.Preview.ReadyCount != 0 || repeated.Preview.DuplicateCount != 1 {
				t.Fatal("moved source was treated as new money", repeated.Preview)
			}
			matches := repeated.Preview.Rows[0].Duplicates
			if len(matches) != 1 || matches[0].TransactionID != corrected.ReplacementID || matches[0].AccountName != cash.Name {
				t.Fatal("wrong moved duplicate", matches)
			}
			correctRecord(t, uid, corrected.ReplacementID, "edit-moved-again", func(d *service.CorrectionDraft) { d.AmountMinor = -300 })
			if _, err := service.NewImportService(pool, q, sumSvc).Commit(ctx, uid, repeated.ID, service.ImportSelection{Rows: []int{2}, DuplicateApprovals: []int{2}}); !errors.Is(err, service.ErrImportChanged) {
				t.Fatal("outdated cross-account duplicate review accepted", err)
			}
		})
	}
}

func TestWorkspaceMigrationDownRetainsCompletedHistory(t *testing.T) {
	ctx := context.Background()
	uid, backup := financialBackupFixture(t)
	raw, err := json.Marshal(backup)
	if err != nil {
		t.Fatal(err)
	}
	target := emptyWorkspaceUser(t)
	svc := service.NewWorkspaceService(pool, q)
	view, err := svc.Preview(ctx, target, "migration.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Restore(ctx, target, view.ID, view.Checksum); err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("../../migrations/000020_workspace_tools.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, string(down)); err == nil || !strings.Contains(err.Error(), "cannot remove correction history") {
		t.Fatal("history rollback guard failed", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	empty, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = empty.Rollback(ctx) }()
	if _, err := empty.Exec(ctx, "DELETE FROM transaction_corrections; DELETE FROM workspace_restores;"); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.Exec(ctx, string(down)); err != nil {
		t.Fatal("empty down path", err)
	}
	if err := empty.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetCorrectionByKey(ctx, sqlc.GetCorrectionByKeyParams{UserID: uid, RequestKey: "move-purchase"}); err != nil {
		t.Fatal("rolled-back DDL lost corrections", err)
	}
	if saved, err := svc.Get(ctx, target, view.ID); err != nil || saved.Receipt == nil {
		t.Fatal("rolled-back DDL lost restore receipt", saved, err)
	}
}

func TestRestoreRejectsStagedPayloadSwappedAfterReview(t *testing.T) {
	ctx := context.Background()
	_, backup := financialBackupFixture(t)
	raw, err := json.Marshal(backup)
	if err != nil {
		t.Fatal(err)
	}
	target := emptyWorkspaceUser(t)
	svc := service.NewWorkspaceService(pool, q)
	view, err := svc.Preview(ctx, target, "reviewed.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	backup.Data.Transactions[0].Note = "Changed after review"
	if err := backup.Seal(); err != nil {
		t.Fatal(err)
	}
	changed, err := json.Marshal(backup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE workspace_restores SET payload=$2::jsonb WHERE id=$1`, view.ID, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Restore(ctx, target, view.ID, view.Checksum); !errors.Is(err, service.ErrBackupInvalid) {
		t.Fatal("unreviewed payload was restored", err)
	}
	empty, err := q.WorkspaceIsEmpty(ctx, target)
	if err != nil || !empty.Bool {
		t.Fatal("changed payload wrote records", empty, err)
	}
}
