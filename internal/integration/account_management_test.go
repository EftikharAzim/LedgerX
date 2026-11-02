package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/planning"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
)

func TestAccountManagementPreservesLedgerAndCreationRetry(t *testing.T) {
	ctx := context.Background()
	uid, _ := newUserWithAccount(t, "USD")
	svc := service.NewAccountService(pool, q)
	in := service.AccountInput{UserID: uid, Name: "Original", Currency: "USD", OpeningMinor: 12345, OpeningAt: time.Now().Add(-time.Hour), IdempotencyKey: "original-account"}
	account, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account.ID, Currency: "USD", AmountMinor: -345, OccurredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	before := importContext(t, uid, account.ID)
	var outbox int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE payload->>'user_id'=$1", fmt.Sprint(uid)).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	rename := service.AccountEdit{Name: ptr("  Everyday  "), ExpectedName: account.Name, ExpectedActive: ptr(true)}
	renamed, err := svc.Update(ctx, uid, account.ID, rename)
	if err != nil || renamed.Name != "Everyday" || renamed.Currency != "USD" || !renamed.IsActive {
		t.Fatalf("rename: %+v %v", renamed, err)
	}
	if _, err := svc.Update(ctx, uid, account.ID, rename); err != nil {
		t.Fatal("lost-response retry", err)
	}
	if _, err := svc.Update(ctx, uid, account.ID, service.AccountEdit{Active: ptr(false), ExpectedName: account.Name, ExpectedActive: ptr(true)}); !errors.Is(err, service.ErrAccountChanged) {
		t.Fatal("stale archive", err)
	}
	archive := service.AccountEdit{Active: ptr(false), ExpectedName: renamed.Name, ExpectedActive: ptr(true)}
	archived, err := svc.Update(ctx, uid, account.ID, archive)
	if err != nil || archived.IsActive {
		t.Fatal("archive", err)
	}
	if _, err := svc.Update(ctx, uid, account.ID, archive); err != nil {
		t.Fatal("archive retry", err)
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account.ID, Currency: "USD", AmountMinor: -100, OccurredAt: time.Now()}); !errors.Is(err, service.ErrAccountInactive) {
		t.Fatal("archived posting", err)
	}
	if after := importContext(t, uid, account.ID); after.BalanceMinor != before.BalanceMinor || after.EntryCount != before.EntryCount {
		t.Fatal("archive changed records", before, after)
	}
	replay, err := svc.Create(ctx, in)
	if err != nil || replay.ID != account.ID || replay.IsActive || replay.Name != renamed.Name {
		t.Fatalf("original creation retry: %+v %v", replay, err)
	}
	backup, err := service.NewWorkspaceService(pool, q).Backup(ctx, uid, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range backup.Data.Accounts {
		if item.ID == account.ID {
			found = true
			if item.Name != renamed.Name || item.Active {
				t.Fatal("backup lost metadata")
			}
		}
	}
	if !found {
		t.Fatal("archived account missing from backup")
	}
	raw, err := json.Marshal(backup)
	if err != nil {
		t.Fatal(err)
	}
	target := emptyWorkspaceUser(t)
	workspace := service.NewWorkspaceService(pool, q)
	preview, err := workspace.Preview(ctx, target, "archived-account.json", raw)
	if err != nil {
		t.Fatal("review archived backup", err)
	}
	if _, err := workspace.Restore(ctx, target, preview.ID, preview.Checksum); err != nil {
		t.Fatal("restore archived backup", err)
	}
	recovered, err := workspace.Backup(ctx, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	recoveredID := int64(0)
	for _, item := range recovered.Data.Accounts {
		if item.Name == renamed.Name {
			if item.Active || item.Currency != "USD" {
				t.Fatal("restore changed account metadata")
			}
			recoveredID = item.ID
		}
	}
	if recoveredID == 0 {
		t.Fatal("archived account missing after restore")
	}
	if got := importContext(t, target, recoveredID); got.BalanceMinor != before.BalanceMinor || got.EntryCount != before.EntryCount {
		t.Fatal("backup restore changed money", got)
	}
	if _, err := svc.Update(ctx, uid, account.ID, service.AccountEdit{Active: ptr(true), ExpectedName: renamed.Name, ExpectedActive: ptr(false)}); err != nil {
		t.Fatal("restore", err)
	}
	if after := importContext(t, uid, account.ID); after.BalanceMinor != 12000 || after.EntryCount != before.EntryCount {
		t.Fatal("restore changed records", after)
	}
	var afterOutbox int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE payload->>'user_id'=$1", fmt.Sprint(uid)).Scan(&afterOutbox); err != nil || outbox != afterOutbox {
		t.Fatal("metadata emitted ledger events", outbox, afterOutbox, err)
	}
}

func TestAccountManagementValidationOwnershipAndHTTP(t *testing.T) {
	ctx := context.Background()
	uid, id := newUserWithAccount(t, "USD")
	_, foreign := newUserWithAccount(t, "USD")
	current, err := q.GetAccount(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewAccountService(pool, q)
	for _, name := range []string{"", " ", strings.Repeat("界", 81), "nul\x00name"} {
		if _, err := svc.Update(ctx, uid, id, service.AccountEdit{Name: &name, ExpectedName: current.Name, ExpectedActive: ptr(true)}); !errors.Is(err, service.ErrAccountEditInput) {
			t.Fatal("invalid name accepted", err)
		}
	}
	for _, edit := range []service.AccountEdit{{}, {Name: ptr("Missing expectation")}, {Name: ptr(current.Name), ExpectedActive: ptr(true)}, {ExpectedName: current.Name, ExpectedActive: ptr(true)}} {
		if _, err := svc.Update(ctx, uid, id, edit); !errors.Is(err, service.ErrAccountEditInput) {
			t.Fatal("incomplete update", err)
		}
	}
	duplicate, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "Duplicate", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	external, err := q.EnsureExternalAccount(ctx, sqlc.EnsureExternalAccountParams{UserID: uid, Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	router := spendingRouter(uid)
	request := func(target int64, body string, status int) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/accounts/%d", target), strings.NewReader(body)))
		if w.Code != status {
			t.Fatalf("PATCH: %d want %d: %s", w.Code, status, w.Body.String())
		}
	}
	for _, target := range []int64{foreign, external, 9223372036854775807} {
		request(target, `{"name":"Hidden","expected_name":"?","expected_active":true}`, 404)
	}
	request(id, `{"currency":"BDT","expected_active":true}`, 400)
	request(id, `{"name":"One","expected_active":true} {}`, 400)
	request(id, `{}`, 422)
	request(id, fmt.Sprintf(`{"name":%q,"expected_active":true}`, current.Name), 422)
	request(id, `{"name":"Other","expected_name":"stale","expected_active":true}`, 409)
	request(id, fmt.Sprintf(`{"name":%q,"expected_name":%q,"expected_active":true}`, duplicate.Name, current.Name), 409)
	request(id, fmt.Sprintf(`{"name":"Renamed via HTTP","expected_name":%q,"expected_active":true}`, current.Name), 200)
	after, err := q.GetAccount(ctx, id)
	if err != nil || after.Name != "Renamed via HTTP" || after.Currency != "USD" || !after.IsActive {
		t.Fatal("HTTP update", after, err)
	}
}

func TestAccountManagementConcurrentEdits(t *testing.T) {
	ctx := context.Background()
	uid, id := newUserWithAccount(t, "USD")
	account, err := q.GetAccount(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewAccountService(pool, q)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"First change", "Second change"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Update(ctx, uid, id, service.AccountEdit{Name: &name, ExpectedName: account.Name, ExpectedActive: ptr(true)})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	applied, stale := 0, 0
	for err := range errs {
		if err == nil {
			applied++
		} else if errors.Is(err, service.ErrAccountChanged) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if applied != 1 || stale != 1 {
		t.Fatal("competing updates", applied, stale)
	}
}

func TestAccountArchiveRecurringAndManualConfirmation(t *testing.T) {
	ctx := context.Background()
	uid, id := newUserWithAccount(t, "USD")
	account, err := q.GetAccount(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	accounts := service.NewAccountService(pool, q)
	schedules := service.NewScheduleService(pool, q)
	plans := service.NewPlanService(pool, q, sumSvc)
	sc, err := schedules.Save(ctx, uid, 0, "archive-bill", scheduleFixture(id, planning.Today(time.Now())))
	if err != nil {
		t.Fatal(err)
	}
	archive := service.AccountEdit{Active: ptr(false), ExpectedName: account.Name, ExpectedActive: ptr(true)}
	if _, err := accounts.Update(ctx, uid, id, archive); !errors.Is(err, service.ErrAccountSchedules) {
		t.Fatal("active bill archive", err)
	}
	paused, err := schedules.Action(ctx, uid, sc.ID, sc.Revision, "pause")
	if err != nil {
		t.Fatal(err)
	}
	var due int64
	if err := pool.QueryRow(ctx, "SELECT id FROM planned_transactions WHERE schedule_id=$1 AND status='pending'", sc.ID).Scan(&due); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.Update(ctx, uid, id, archive); err != nil {
		t.Fatal(err)
	}
	if _, err := schedules.Action(ctx, uid, sc.ID, paused.Revision, "resume"); !errors.Is(err, service.ErrAccountInactive) {
		t.Fatal("resumed archived account", err)
	}
	if _, err := plans.Resolve(ctx, uid, due, "", false); !errors.Is(err, service.ErrAccountInactive) {
		t.Fatal("confirmed archived plan", err)
	}
	var status string
	if err := pool.QueryRow(ctx, "SELECT status FROM planned_transactions WHERE id=$1", due).Scan(&status); err != nil || status != "pending" {
		t.Fatal("plan was lost", err)
	}
	if _, err := accounts.Update(ctx, uid, id, service.AccountEdit{Active: ptr(true), ExpectedName: account.Name, ExpectedActive: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT status FROM recurring_schedules WHERE id=$1", sc.ID).Scan(&status); err != nil || status != "paused" {
		t.Fatal("restore resumed bill", err)
	}
	if got := importContext(t, uid, id); got.BalanceMinor != 0 || got.EntryCount != 0 {
		t.Fatal("metadata posted a plan", got)
	}
	if _, err := plans.Resolve(ctx, uid, due, "", false); err != nil {
		t.Fatal("confirm after restore", err)
	}
	if _, err := schedules.Action(ctx, uid, sc.ID, paused.Revision, "resume"); err != nil {
		t.Fatal("explicit resume after restore", err)
	}
}
