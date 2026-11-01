package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/planning"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
)

func scheduleFixture(account int64, day time.Time) service.ScheduleInput {
	return service.ScheduleInput{AccountID: account, Currency: "USD", AmountMinor: -900, Note: "Synthetic weekly bill", Cadence: "weekly", FirstDueDate: day.Format("2006-01-02")}
}

func TestRecurringCreateSyncSkipPauseResumeAndEnd(t *testing.T) {
	ctx := context.Background()
	today := planning.Today(time.Now())
	uid, account := newUserWithAccount(t, "USD")
	svc := service.NewScheduleService(pool, q)
	input := scheduleFixture(account, today.AddDate(0, 0, 1))
	ids := make([]int64, 6)
	errs := make([]error, 6)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc, err := svc.Save(ctx, uid, 0, "same-schedule", input)
			ids[i], errs[i] = sc.ID, err
		}()
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] != ids[0] || ids[i] == 0 {
			t.Fatal("schedule create replay", errs[i])
		}
	}
	if got := importContext(t, uid, account); got.BalanceMinor != 0 || got.EntryCount != 0 {
		t.Fatal("recurring creation posted money")
	}
	input.AmountMinor--
	if _, err := svc.Save(ctx, uid, 0, "same-schedule", input); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatal("changed retry accepted", err)
	}
	input.AmountMinor++
	var first int64
	var initial int
	if err := pool.QueryRow(ctx, "SELECT min(id),count(*) FROM planned_transactions WHERE schedule_id=$1", ids[0]).Scan(&first, &initial); err != nil {
		t.Fatal(err)
	}
	planSvc := service.NewPlanService(pool, q, sumSvc)
	if _, err := planSvc.Resolve(ctx, uid, first, "", true); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(ctx, uid)
	if err != nil || len(list) != 1 {
		t.Fatal("schedule list", err)
	}
	sc := list[0].Schedule
	foreign, _ := newUserWithAccount(t, "USD")
	if _, err := svc.Action(ctx, foreign, sc.ID, sc.Revision, "pause"); !errors.Is(err, service.ErrScheduleNotFound) {
		t.Fatal("foreign schedule action", err)
	}
	paused, err := svc.Action(ctx, uid, sc.ID, sc.Revision, "pause")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Action(ctx, uid, sc.ID, sc.Revision, "pause"); err != nil {
		t.Fatal("pause retry", err)
	}
	var pending int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM planned_transactions WHERE schedule_id=$1 AND status='pending'", sc.ID).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("paused future obligations remained", pending, err)
	}
	resumed, err := svc.Action(ctx, uid, sc.ID, paused.Revision, "resume")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Sync(ctx, uid, time.Now()); err != nil {
		t.Fatal(err)
	}
	var total int
	var status, reason string
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM planned_transactions WHERE schedule_id=$1", sc.ID).Scan(&total); err != nil || total != initial {
		t.Fatal("sync duplicated dates", total, initial, err)
	}
	if err := pool.QueryRow(ctx, "SELECT status,cancellation_reason FROM planned_transactions WHERE id=$1", first).Scan(&status, &reason); err != nil || status != "canceled" || reason != "skipped" {
		t.Fatal("resume revived a skipped date", err)
	}
	if _, err := svc.Action(ctx, uid, sc.ID, sc.Revision, "pause"); !errors.Is(err, service.ErrScheduleChanged) {
		t.Fatal("stale action accepted", err)
	}
	ended, err := svc.Action(ctx, uid, sc.ID, resumed.Revision, "end")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Action(ctx, uid, sc.ID, ended.Revision, "resume"); !errors.Is(err, service.ErrScheduleEnded) {
		t.Fatal("ended schedule restarted", err)
	}
	if got := importContext(t, uid, account); got.BalanceMinor != 0 || got.EntryCount != 0 {
		t.Fatal("control changed money")
	}
}

func TestRecurringPaymentsForecastAndPortableRecovery(t *testing.T) {
	ctx := context.Background()
	today := planning.Today(time.Now())
	uid, account := newUserWithAccount(t, "USD")
	svc := service.NewScheduleService(pool, q)
	plans := service.NewPlanService(pool, q, sumSvc)
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", AmountMinor: 10000, OccurredAt: today}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "Separate currency", Currency: "BDT"}); err != nil {
		t.Fatal(err)
	}
	bill := scheduleFixture(account, today.AddDate(0, 0, 2))
	sc, err := svc.Save(ctx, uid, 0, "weekly-forecast", bill)
	if err != nil {
		t.Fatal(err)
	}
	income := scheduleFixture(account, today.AddDate(0, 0, 4))
	income.Cadence = "monthly"
	income.EndDate = income.FirstDueDate
	income.AmountMinor = 5000
	income.Note = "Synthetic expected income"
	if _, err := svc.Save(ctx, uid, 0, "income-forecast", income); err != nil {
		t.Fatal(err)
	}
	overdue := scheduleFixture(account, today.AddDate(0, 0, -1))
	overdue.EndDate = overdue.FirstDueDate
	overdue.AmountMinor = -300
	overdue.Note = "Synthetic overdue bill"
	old, err := svc.Save(ctx, uid, 0, "overdue-forecast", overdue)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := svc.Cashflow(ctx, uid, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(flow.Wallets) != 2 {
		t.Fatal("currencies merged")
	}
	w := flow.Wallets[0]
	if w.CurrentMinor != 10000 || w.InMinor != 5000 || w.OutMinor != 3900 || w.ProjectedMinor != 11100 || w.LowestMinor != 8800 || w.OverdueCount != 1 {
		t.Fatalf("forecast %+v", w)
	}
	var first, oldPlan int64
	if err := pool.QueryRow(ctx, "SELECT min(id) FROM planned_transactions WHERE schedule_id=$1", sc.ID).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT min(id) FROM planned_transactions WHERE schedule_id=$1", old.ID).Scan(&oldPlan); err != nil {
		t.Fatal(err)
	}
	if _, err := plans.Resolve(ctx, uid, first, "", true); err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 6)
	errs := make([]error, 6)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], errs[i] = plans.Resolve(ctx, uid, oldPlan, today.Format("2006-01-02"), false)
		}()
	}
	wg.Wait()
	for i := range ids {
		if errs[i] != nil || ids[i] == 0 || ids[i] != ids[0] {
			t.Fatal("payment posted twice", errs[i])
		}
	}
	after, err := svc.Cashflow(ctx, uid, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if after.Wallets[0].CurrentMinor != 9700 || after.Wallets[0].ProjectedMinor != 12000 || after.Wallets[0].OverdueCount != 0 {
		t.Fatal("confirm did not reconcile projection", after.Wallets[0])
	}
	bs := service.NewWorkspaceService(pool, q)
	b, err := bs.Backup(ctx, uid, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(b)
	for _, field := range []string{"creation_key", "creation_hash", "last_edit_key", "last_edit_hash"} {
		if strings.Contains(string(raw), field) {
			t.Fatal("backup contains retry data", field)
		}
	}
	target := emptyWorkspaceUser(t)
	view, err := bs.Preview(ctx, target, "recurring-backup.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if view.Summary.ScheduleCount != 3 {
		t.Fatal("schedules missing from preview")
	}
	if _, err := bs.Restore(ctx, target, view.ID, view.Checksum); err != nil {
		t.Fatal(err)
	}
	restored, err := svc.List(ctx, target)
	if err != nil || len(restored) != 3 {
		t.Fatal("restored schedules", err)
	}
	for _, v := range restored {
		if v.ID == sc.ID || v.ID == old.ID {
			t.Fatal("schedule ID not remapped")
		}
	}
	before := len(b.Data.Plans)
	if err := svc.Sync(ctx, target, time.Now()); err != nil {
		t.Fatal(err)
	}
	again, err := bs.Backup(ctx, target, nil)
	if err != nil || len(again.Data.Plans) != before {
		t.Fatal("restore/sync duplicated occurrences", err)
	}
}

func TestRecurringEditsPreserveEarlierPlansAndSkippedDates(t *testing.T) {
	ctx := context.Background()
	today := planning.Today(time.Now())
	uid, account := newUserWithAccount(t, "USD")
	svc := service.NewScheduleService(pool, q)
	in := scheduleFixture(account, today.AddDate(0, 0, -1))
	sc, err := svc.Save(ctx, uid, 0, "edit-series", in)
	if err != nil {
		t.Fatal(err)
	}
	var due, future int64
	if err := pool.QueryRow(ctx, "SELECT min(id),max(id) FROM planned_transactions WHERE schedule_id=$1", sc.ID).Scan(&due, &future); err != nil {
		t.Fatal(err)
	}
	if _, err := service.NewPlanService(pool, q, sumSvc).Resolve(ctx, uid, future, "", true); err != nil {
		t.Fatal(err)
	}
	in.AmountMinor = -1200
	in.Revision = sc.Revision
	edited, err := svc.Save(ctx, uid, sc.ID, "edit-one", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(ctx, uid, sc.ID, "edit-one", in); err != nil {
		t.Fatal("edit replay", err)
	}
	var original int64
	var reason string
	if err := pool.QueryRow(ctx, "SELECT amount_minor FROM planned_transactions WHERE id=$1", due).Scan(&original); err != nil || original != -900 {
		t.Fatal("earlier obligation changed", err)
	}
	if err := pool.QueryRow(ctx, "SELECT cancellation_reason FROM planned_transactions WHERE id=$1", future).Scan(&reason); err != nil || reason != "skipped" {
		t.Fatal("edit revived skipped date", err)
	}
	if edited.Revision != sc.Revision+1 {
		t.Fatal("revision not incremented")
	}
	in.Revision = edited.Revision
	in.Cadence = "monthly"
	if _, err := svc.Save(ctx, uid, sc.ID, "bad-pattern", in); !errors.Is(err, service.ErrScheduleInput) {
		t.Fatal("pattern backfilled history", err)
	}
}

func TestRecurringPauseGapKeepsMonthEndAnchor(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	clock, _ := planning.Date("2024-01-15")
	svc := service.NewScheduleService(pool, q, func() time.Time { return clock })
	in := scheduleFixture(account, time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC))
	in.Cadence = "monthly"
	sc, err := svc.Save(ctx, uid, 0, "month-end-gap", in)
	if err != nil {
		t.Fatal(err)
	}
	clock, _ = planning.Date("2024-02-01")
	paused, err := svc.Action(ctx, uid, sc.ID, sc.Revision, "pause")
	if err != nil {
		t.Fatal(err)
	}
	clock, _ = planning.Date("2024-03-15")
	resumed, err := svc.Action(ctx, uid, sc.ID, paused.Revision, "resume")
	if err != nil {
		t.Fatal(err)
	}
	var status, reason string
	if err := pool.QueryRow(ctx, "SELECT status,cancellation_reason FROM planned_transactions WHERE schedule_id=$1 AND scheduled_date='2024-02-29'", sc.ID).Scan(&status, &reason); err != nil || status != "canceled" || reason != "paused" {
		t.Fatal("paused-period date came back", status, reason, err)
	}
	for _, date := range []string{"2024-01-31", "2024-03-31", "2024-04-30", "2024-05-31"} {
		if err := pool.QueryRow(ctx, "SELECT status FROM planned_transactions WHERE schedule_id=$1 AND scheduled_date=$2::date", sc.ID, date).Scan(&status); err != nil || status != "pending" {
			t.Fatal("anchor drift or earlier due plan lost", date, status, err)
		}
	}
	if _, err := svc.Action(ctx, uid, sc.ID, resumed.Revision, "end"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT status FROM planned_transactions WHERE schedule_id=$1 AND scheduled_date='2024-01-31'", sc.ID).Scan(&status); err != nil || status != "pending" {
		t.Fatal("ending discarded an already-due obligation", err)
	}
}

func TestRecurringCategoryReassignmentAndStaleEdit(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	cat := managedCategory(t, uid, 0, "Bills", "expense")
	replacement := managedCategory(t, uid, 0, "Home", "expense")
	in := scheduleFixture(account, planning.Today(time.Now()).AddDate(0, 0, 1))
	in.CategoryID = cat.ID
	svc := service.NewScheduleService(pool, q)
	sc, err := svc.Save(ctx, uid, 0, "category-series", in)
	if err != nil {
		t.Fatal(err)
	}
	cats := service.NewCategoryService(pool, q)
	impact, err := cats.Impact(ctx, uid, cat.ID)
	if err != nil || impact.Schedules != 1 {
		t.Fatal("recurring impact omitted", err)
	}
	if err := cats.Delete(ctx, uid, cat.ID, replacement.ID, impact.Token); err != nil {
		t.Fatal(err)
	}
	in.Revision = sc.Revision
	in.Note = "Stale edit"
	if _, err := svc.Save(ctx, uid, sc.ID, "stale-series", in); !errors.Is(err, service.ErrScheduleChanged) {
		t.Fatal("category reassignment accepted stale template", err)
	}
	current, err := svc.List(ctx, uid)
	if err != nil || len(current) != 1 || current[0].CategoryID != replacement.ID {
		t.Fatal("future schedule category was not reassigned", err)
	}
	if _, err := service.NewWorkspaceService(pool, q).Backup(ctx, uid, nil); err != nil {
		t.Fatal("reassigned recurring backup failed", err)
	}
}

func TestRecurringConfirmationUsesActualAmountAndStartingDate(t *testing.T) {
	ctx := context.Background()
	today := planning.Today(time.Now())
	uid, _ := newUserWithAccount(t, "USD")
	account, err := service.NewAccountService(pool, q).Create(ctx, service.AccountInput{UserID: uid, Name: "Actual payment wallet", Currency: "USD", OpeningMinor: 10000, OpeningAt: today.AddDate(0, 0, -3), IdempotencyKey: "actual-opening"})
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewScheduleService(pool, q)
	sc, err := svc.Save(ctx, uid, 0, "actual-schedule", scheduleFixture(account.ID, today))
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := pool.QueryRow(ctx, "SELECT min(id) FROM planned_transactions WHERE schedule_id=$1", sc.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	plans := service.NewPlanService(pool, q, sumSvc)
	actual := int64(-950)
	if _, err := plans.ResolveAmount(ctx, uid, id, today.Format("2006-01-02")+"T00:00:00Z", false, &actual); !errors.Is(err, service.ErrPlanInput) {
		t.Fatal("non-calendar actual date accepted", err)
	}
	if _, err := plans.ResolveAmount(ctx, uid, id, today.AddDate(0, 0, -4).Format("2006-01-02"), false, &actual); !errors.Is(err, service.ErrPlanDate) {
		t.Fatal("payment before starting money accepted", err)
	}
	first, err := plans.ResolveAmount(ctx, uid, id, today.Format("2006-01-02"), false, &actual)
	if err != nil {
		t.Fatal(err)
	}
	again, err := plans.ResolveAmount(ctx, uid, id, today.Format("2006-01-02"), false, &actual)
	if err != nil || again != first {
		t.Fatal("actual payment retry", err)
	}
	different := actual - 1
	if _, err := plans.ResolveAmount(ctx, uid, id, today.Format("2006-01-02"), false, &different); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatal("changed actual amount silently accepted", err)
	}
	if got := importContext(t, uid, account.ID); got.BalanceMinor != 9050 {
		t.Fatal("estimate posted instead of actual", got.BalanceMinor)
	}
	b, err := service.NewWorkspaceService(pool, q).Backup(ctx, uid, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range b.Data.Plans {
		if p.ID == id && (p.AmountMinor != -900 || p.RecordedAmountMinor == nil || *p.RecordedAmountMinor != -950) {
			t.Fatal("planned estimate or actual was lost")
		}
	}
	raw, _ := json.Marshal(b)
	target := emptyWorkspaceUser(t)
	bs := service.NewWorkspaceService(pool, q)
	v, err := bs.Preview(ctx, target, "actual-backup.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bs.Restore(ctx, target, v.ID, v.Checksum); err != nil {
		t.Fatal(err)
	}
}
