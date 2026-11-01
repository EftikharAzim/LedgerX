package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/reporting"
	"github.com/EftikharAzim/ledgerx/internal/service"
)

func ptr[T any](v T) *T { return &v }
func managedCategory(t *testing.T, uid, parent int64, name, kind string) sqlc.Category {
	t.Helper()
	c, err := service.NewCategoryService(pool, q).Save(context.Background(), uid, 0, service.CategoryEdit{Name: ptr(name), ParentID: ptr(parent), Kind: ptr(kind), Icon: ptr("food")})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCategoryHierarchyArchiveReassignmentAndRecovery(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	svc := service.NewCategoryService(pool, q)
	parent := managedCategory(t, uid, 0, "Food", "expense")
	child := managedCategory(t, uid, parent.ID, "Groceries", "expense")
	inherited, err := svc.Save(ctx, uid, 0, service.CategoryEdit{Name: ptr("Fruit"), ParentID: ptr(parent.ID)})
	if err != nil || inherited.Kind != "expense" {
		t.Fatal("subcategory did not inherit its parent's type", err)
	}
	if _, err := svc.Save(ctx, uid, parent.ID, service.CategoryEdit{ParentID: ptr(child.ID)}); !errors.Is(err, service.ErrCategoryInput) {
		t.Fatal("cycle accepted", err)
	}
	if _, err := svc.Save(ctx, uid, 0, service.CategoryEdit{Name: ptr("Third level"), ParentID: ptr(child.ID)}); !errors.Is(err, service.ErrCategoryInput) {
		t.Fatal("third level accepted", err)
	}
	foreign, _ := newUserWithAccount(t, "USD")
	if _, err := svc.Save(ctx, foreign, child.ID, service.CategoryEdit{Name: ptr("Foreign edit")}); err == nil {
		t.Fatal("foreign category edited")
	}
	entry, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", AmountMinor: -1000, CategoryID: child.ID, OccurredAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.NewPlanService(pool, q, sumSvc).Create(ctx, uid, "planned-groceries", service.PlanInput{AccountID: account, Currency: "USD", AmountMinor: -300, CategoryID: child.ID, OccurredAt: time.Now().AddDate(0, 1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	before := importContext(t, uid, account)
	if _, err := svc.Save(ctx, uid, parent.ID, service.CategoryEdit{Archived: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	selectable, err := q.ListSelectableCategoriesByUser(ctx, uid)
	if err != nil || len(selectable) != 0 {
		t.Fatal("archived tree remained selectable", err)
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", AmountMinor: -1, CategoryID: child.ID, OccurredAt: time.Now()}); !errors.Is(err, service.ErrInvalidCategory) {
		t.Fatal("archived category accepted", err)
	}
	if _, err := svc.Save(ctx, uid, parent.ID, service.CategoryEdit{Archived: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(ctx, uid, child.ID, service.CategoryEdit{Archived: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	// Correction receipts retain the old category identity after reassignment.
	correctRecord(t, uid, entry.ID, "category-correction", func(d *service.CorrectionDraft) { d.Note = "Corrected grocery note" })
	impact, err := svc.Impact(ctx, uid, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.Transactions != 2 || impact.Plans != 1 {
		t.Fatal("impact omitted financial history", impact)
	}
	if _, err := svc.Save(ctx, uid, child.ID, service.CategoryEdit{Name: ptr("Groceries renamed")}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, uid, child.ID, parent.ID, impact.Token); !errors.Is(err, service.ErrCategoryChanged) {
		t.Fatal("stale category review accepted", err)
	}
	impact, err = svc.Impact(ctx, uid, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, uid, child.ID, parent.ID, impact.Token); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, uid, child.ID, parent.ID, impact.Token); err != nil {
		t.Fatal("deletion retry", err)
	}
	after := importContext(t, uid, account)
	if before.BalanceMinor != after.BalanceMinor {
		t.Fatal("category deletion changed money")
	}
	var untouched int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE user_id=$1 AND category_id=$2", uid, child.ID).Scan(&untouched); err != nil || untouched != 0 {
		t.Fatal("category references not moved", err)
	}
	// A deleted label can be reused, while receipts retain its previous identity.
	managedCategory(t, uid, 0, "Groceries renamed", "expense")
	bs := service.NewWorkspaceService(pool, q)
	b, err := bs.Backup(ctx, uid, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(b)
	target := emptyWorkspaceUser(t)
	v, err := bs.Preview(ctx, target, "categories.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bs.Restore(ctx, target, v.ID, v.Checksum); err != nil {
		t.Fatal(err)
	}
	if _, err := bs.Backup(ctx, target, nil); err != nil {
		t.Fatal("restored history invalid", err)
	}
}

func TestMonthlyBudgetsParentRefundsExclusionsAndPlans(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	parent := managedCategory(t, uid, 0, "Food", "expense")
	child := managedCategory(t, uid, parent.ID, "Groceries", "expense")
	record := func(amount int64, kind string, cat int64, month time.Month) int64 {
		t.Helper()
		v, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", AmountMinor: amount, EntryType: kind, CategoryID: cat, OccurredAt: time.Date(2024, month, 4, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		return v.ID
	}
	record(-1000, "standard", child.ID, time.January)
	record(-300, "standard", parent.ID, time.January)
	record(200, "refund", child.ID, time.January)
	record(-900, "standard", child.ID, time.February)
	excluded := importInput(account, "Date,Amount,Note,Category,Reference,Exclude Report\n2024-01-06,-7,Excluded,Groceries,excluded,true")
	excluded.Mapping.ExcludeReport = importColumn(5)
	importCommit(t, uid, importPreview(t, uid, excluded))
	servicePlan := service.NewPlanService(pool, q, sumSvc)
	if _, err := servicePlan.Create(ctx, uid, "future-limit", service.PlanInput{AccountID: account, AmountMinor: -999, Currency: "USD", CategoryID: child.ID, OccurredAt: time.Now().AddDate(0, 1, 0)}); err != nil {
		t.Fatal(err)
	}
	budgets := service.NewBudgetService(pool, q)
	ids := map[int64]int64{}
	for _, scope := range []int64{0, parent.ID, child.ID} {
		id, err := budgets.Save(ctx, uid, service.BudgetInput{Currency: "USD", Month: "2024-01", CategoryID: scope, AmountMinor: 1000})
		if err != nil {
			t.Fatal(err)
		}
		ids[scope] = id
	}
	rows, err := budgets.List(ctx, uid, "2024-01", "USD")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatal("missing budgets")
	}
	for _, b := range rows {
		want := int64(1100)
		if b.CategoryID == child.ID {
			want = 800
		}
		if b.SpentMinor != want || b.RemainingMinor != 1000-want {
			t.Fatalf("scope %d spent %d remaining %d", b.CategoryID, b.SpentMinor, b.RemainingMinor)
		}
	}
	if id, err := budgets.Save(ctx, uid, service.BudgetInput{Currency: "USD", Month: "2024-01", CategoryID: parent.ID, AmountMinor: 2000}); err != nil || id != ids[parent.ID] {
		t.Fatal("budget retry created duplicates", err)
	}
	impact, err := service.NewCategoryService(pool, q).Impact(ctx, uid, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.NewCategoryService(pool, q).Delete(ctx, uid, child.ID, parent.ID, impact.Token); !errors.Is(err, service.ErrCategoryInUse) {
		t.Fatal("budget category silently merged", err)
	}
	if n, err := budgets.Copy(ctx, uid, "2024-01", "2024-02", "USD"); err != nil || n != 3 {
		t.Fatal("budget copy", n, err)
	}
	if n, err := budgets.Copy(ctx, uid, "2024-01", "2024-02", "USD"); err != nil || n != 0 {
		t.Fatal("copy retry replaced limits", n, err)
	}
	foreign, _ := newUserWithAccount(t, "USD")
	if err := budgets.Delete(ctx, foreign, ids[0]); !errors.Is(err, service.ErrBudgetNotFound) {
		t.Fatal("foreign budget deleted", err)
	}
	bs := service.NewWorkspaceService(pool, q)
	b, err := bs.Backup(ctx, uid, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Data.Budgets) != 6 {
		t.Fatal("budgets missing in backup")
	}
	raw, _ := json.Marshal(b)
	target := emptyWorkspaceUser(t)
	v, err := bs.Preview(ctx, target, "budget-backup.json", raw)
	if err != nil {
		t.Fatal(err)
	}
	if v.Summary.BudgetCount != 6 {
		t.Fatal("missing restore summary")
	}
	if _, err := bs.Restore(ctx, target, v.ID, v.Checksum); err != nil {
		t.Fatal(err)
	}
	restored, err := budgets.List(ctx, target, "2024-01", "USD")
	if err != nil || len(restored) != 3 {
		t.Fatal("budgets not restored", err)
	}
	for _, b := range restored {
		if b.ID == ids[0] || b.ID == ids[parent.ID] || b.ID == ids[child.ID] {
			t.Fatal("budget identity not remapped")
		}
	}
}

func TestFinancialReportBalancesDrilldownAndCalendarComparison(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	category := managedCategory(t, uid, 0, "Everyday", "any")
	for _, v := range []struct {
		at     string
		amount int64
		kind   string
	}{{"2023-12-31", 10000, "standard"}, {"2024-01-01", 2000, "standard"}, {"2024-01-03", -600, "standard"}, {"2024-01-04", 100, "refund"}, {"2024-02-01", -100, "standard"}} {
		at, _ := time.Parse("2006-01-02", v.at)
		if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", AmountMinor: v.amount, OccurredAt: at, EntryType: v.kind, CategoryID: category.ID}); err != nil {
			t.Fatal(err)
		}
	}
	excluded := importInput(account, "Date,Amount,Note,Category,Reference,Exclude Report\n2024-01-06,-7,Excluded,Everyday,excluded-report,true")
	excluded.Mapping.ExcludeReport = importColumn(5)
	importCommit(t, uid, importPreview(t, uid, excluded))
	svc := service.NewFinancialReportService(pool, q)
	period, _ := reporting.Parse(reporting.Selection{Type: "monthly", Month: "2024-01"})
	r, err := svc.Get(ctx, uid, "USD", period)
	if err != nil {
		t.Fatal(err)
	}
	if r.Opening != 10000 || r.Ending != 10800 || r.Income != 2000 || r.Spending != 500 || r.Net != 1500 || r.Other != -700 {
		t.Fatalf("report reconciliation %+v", r)
	}
	if r.Previous == nil || r.Previous.Income != 10000 || r.PreviousStart != "2023-12-01" || r.PreviousEnd != "2023-12-31" || len(r.Trends) != 6 {
		t.Fatal("calendar comparison", r)
	}
	if len(r.Categories) != 1 || r.Categories[0].Income != 2000 || r.Categories[0].Spending != 500 {
		t.Fatal("income/spending drilldown", r.Categories)
	}
	for _, kind := range []string{"weekly", "yearly", "custom", "all"} {
		selection := reporting.Selection{Type: kind}
		switch kind {
		case "weekly":
			selection.WeekOf = "2024-01-03"
		case "yearly":
			selection.Year = "2024"
		case "custom":
			selection.StartDate = "2024-01-01"
			selection.EndDate = "2024-01-31"
		}
		p, err := reporting.Parse(selection)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Get(ctx, uid, "USD", p); err != nil {
			t.Fatalf("%s report: %v", kind, err)
		}
	}
}

func TestMoneyManagementHTTPValidationAndParentActivity(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	parent := managedCategory(t, uid, 0, "Home", "expense")
	child := managedCategory(t, uid, parent.ID, "Rent", "expense")
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", AmountMinor: -50, CategoryID: child.ID, OccurredAt: time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	var activity struct {
		Entries []map[string]any `json:"entries"`
	}
	getWorkspace(t, importsRouter(uid), fmt.Sprintf("/activity?category_id=%d", parent.ID), &activity)
	if len(activity.Entries) != 1 {
		t.Fatal("parent activity omitted subcategory")
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), httptransport.UserIDKey, uid)))
		})
	})
	httptransport.NewMoneyManagementAPI(service.NewBudgetService(pool, q), service.NewFinancialReportService(pool, q)).Routes(r)
	httptransport.NewCategoriesAPI(q, service.NewCategoryService(pool, q)).Routes(r)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/reports/summary?currency=USD&type=monthly&month=2024-01", "", 200},
		{"GET", "/reports/summary?currency=usd&type=monthly&month=2024-01", "", 400},
		{"GET", "/reports/summary?currency=USD&type=monthly&month=bad", "", 400},
		{"PUT", "/budgets", `{"month":"2024-01","currency":"USD","amount_minor":0}`, 422},
		{"PUT", "/budgets", `{"month":"2024-13","currency":"USD","amount_minor":100}`, 422},
		{"PATCH", "/categories/invalid", `{"name":"No accidental creation"}`, 400},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
