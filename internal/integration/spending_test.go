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

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func spendingRouter(uid int64) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), httptransport.UserIDKey, uid)))
		})
	})
	httptransport.NewAccountsAPI(q, service.NewAccountService(pool, q)).Routes(r)
	httptransport.NewSpendingAPI(q).Routes(r)
	httptransport.NewTransactionsAPI(q, txSvc).Routes(r)
	return r
}

func getWorkspace(t *testing.T, router http.Handler, path string, body any) {
	t.Helper()
	r := httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
	if r.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, r.Code, r.Body.String())
	}
	if err := json.Unmarshal(r.Body.Bytes(), body); err != nil {
		t.Fatal(err)
	}
}

func TestSpendingRealLifeScenarios(t *testing.T) {
	ctx := context.Background()
	uid, target := newUserWithAccount(t, "USD")
	svc := service.NewAccountService(pool, q)
	jan := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	account, err := svc.Create(ctx, service.AccountInput{UserID: uid, Name: "Everyday", Currency: "USD", OpeningMinor: 10000, OpeningAt: jan.AddDate(0, 0, -14), IdempotencyKey: "starting-money"})
	if err != nil {
		t.Fatal(err)
	}
	cat, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: uid, Name: "Clothing"})
	if err != nil {
		t.Fatal(err)
	}
	record := func(amount int64, entryType string, category int64, at time.Time) service.TransactionDTO {
		t.Helper()
		tx, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account.ID, AmountMinor: amount, Currency: "USD", OccurredAt: at, CategoryID: category, EntryType: entryType})
		if err != nil {
			t.Fatal(err)
		}
		return tx
	}
	record(50000, "", 0, jan)
	record(-1000, "", cat.ID, jan)
	refund := record(400, "refund", cat.ID, jan)
	record(-200, "", 0, jan)
	if _, err := txSvc.Transfer(ctx, service.TransferInput{UserID: uid, FromAccountID: account.ID, ToAccountID: target, AmountMinor: 10000, Currency: "USD", OccurredAt: jan}); err != nil {
		t.Fatal(err)
	}
	for _, date := range []time.Time{jan, jan.AddDate(0, 1, 0), time.Now().UTC()} {
		original := record(-300, "", cat.ID, date)
		if _, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: original.ID}); err != nil {
			t.Fatal(err)
		}
	}
	// A later refund can make a category's net spending negative.
	record(150, "refund", cat.ID, jan.AddDate(0, 2, 0))
	foreignUID, foreignAccount := newUserWithAccount(t, "USD")
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: foreignUID, AccountID: foreignAccount, AmountMinor: -999999, Currency: "USD", OccurredAt: jan}); err != nil {
		t.Fatal(err)
	}
	jpy, err := svc.Create(ctx, service.AccountInput{UserID: uid, Name: "Yen", Currency: "JPY", OpeningMinor: 500, OpeningAt: jan})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: jpy.ID, AmountMinor: -25, Currency: "JPY", OccurredAt: jan}); err != nil {
		t.Fatal(err)
	}
	router := spendingRouter(uid)
	var summary struct {
		Income     int64 `json:"income_minor"`
		Spending   int64 `json:"spending_minor"`
		Net        int64 `json:"net_minor"`
		Categories []struct {
			ID     int64 `json:"category_id"`
			Amount int64 `json:"spending_minor"`
		} `json:"categories"`
	}
	getWorkspace(t, router, "/spending?type=monthly&month=2024-01&currency=USD", &summary)
	if summary.Income != 50000 || summary.Spending != 800 || summary.Net != 49200 || len(summary.Categories) != 2 {
		t.Fatalf("spending included starting money, transfers, corrections, another currency, or another owner: %+v", summary)
	}
	var categorySum int64
	for _, c := range summary.Categories {
		categorySum += c.Amount
	}
	if categorySum != summary.Spending {
		t.Fatal("category totals disagree with spending")
	}
	getWorkspace(t, router, "/spending?type=monthly&month=2024-02&currency=USD", &summary)
	if summary.Income != 0 || summary.Spending != 0 {
		t.Fatalf("cross-period reversal must restate original period: %+v", summary)
	}
	getWorkspace(t, router, "/spending?type=monthly&month=2024-03&currency=USD", &summary)
	if summary.Income != 0 || summary.Spending != -150 {
		t.Fatalf("refund-only month became income or was clamped: %+v", summary)
	}
	getWorkspace(t, router, "/spending?type=yearly&year=2024&currency=JPY", &summary)
	if summary.Income != 0 || summary.Spending != 25 {
		t.Fatalf("currencies mixed: %+v", summary)
	}
	getWorkspace(t, router, "/spending?type=monthly&month="+time.Now().UTC().Format("2006-01")+"&currency=USD", &summary)
	if summary.Income != 0 || summary.Spending != 0 {
		t.Fatalf("same-period reversal became income or spending: %+v", summary)
	}
	var uncategorized struct {
		Entries []struct {
			Type string `json:"activity_type"`
		} `json:"entries"`
	}
	getWorkspace(t, router, "/activity?category_id=0&limit=100", &uncategorized)
	if len(uncategorized.Entries) != 3 {
		t.Fatalf("uncategorized review contains irrelevant ledger movements: %+v", uncategorized)
	}
	for _, entry := range uncategorized.Entries {
		if entry.Type != "income" && entry.Type != "expense" && entry.Type != "refund" {
			t.Fatalf("special entry needs categorization: %+v", entry)
		}
	}
	if _, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: refund.ID}); err != nil {
		t.Fatal(err)
	}
	getWorkspace(t, router, "/spending?type=monthly&month=2024-01&currency=USD", &summary)
	if summary.Income != 50000 || summary.Spending != 1200 {
		t.Fatalf("reversed refund still changed spending: %+v", summary)
	}
}

func TestSpendingDateBoundariesIgnoreDatabaseTimeZone(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	for _, example := range []struct {
		at     string
		amount int64
		kind   string
	}{
		{"2024-01-31T23:59:59.999999Z", 999, ""},
		{"2024-02-01T00:00:00Z", -100, ""},
		{"2024-02-29T23:59:59.999999Z", 25, "refund"},
		{"2024-03-01T00:00:00Z", 999, ""},
	} {
		at, err := time.Parse(time.RFC3339Nano, example.at)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: example.amount, Currency: "USD", OccurredAt: at, EntryType: example.kind}); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL TIME ZONE 'Asia/Dhaka'"); err != nil {
		t.Fatal(err)
	}
	rows, err := q.WithTx(tx).GetSpendingByCategory(ctx, sqlc.GetSpendingByCategoryParams{UserID: uid, Currency: "USD", StartDate: pgtype.Date{Time: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Valid: true}, EndDate: pgtype.Date{Time: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), Valid: true}})
	if err != nil || len(rows) != 1 {
		t.Fatalf("UTC boundary query: %+v %v", rows, err)
	}
	if rows[0].IncomeMinor != 0 || rows[0].SpendingMinor != 75 || rows[0].SpendingCount != 2 {
		t.Fatalf("wrong UTC inclusive boundaries: %+v", rows)
	}
}

func TestStartingBalanceReplayConcurrencyAndRollback(t *testing.T) {
	ctx := context.Background()
	uid, _ := newUserWithAccount(t, "USD")
	svc := service.NewAccountService(pool, q)
	in := service.AccountInput{UserID: uid, Name: "Starting debt", Currency: "USD", OpeningMinor: -12345, OpeningAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), IdempotencyKey: "same-account"}
	var wg sync.WaitGroup
	results := make(chan int64, 8)
	errorsSeen := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, err := svc.Create(ctx, in)
			if err != nil {
				errorsSeen <- err
			} else {
				results <- row.ID
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		t.Fatalf("concurrent account retry failed: %v", err)
	}
	var id int64
	for got := range results {
		if id != 0 && id != got {
			t.Fatal("retry created another account")
		}
		id = got
	}
	balance, err := balSvc.CurrentBalance(ctx, id, time.Now())
	if err != nil || balance.Balance != -12345 {
		t.Fatalf("starting balance duplicated or missing: %+v %v", balance, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM postings WHERE account_id=$1", id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("starting entry count %d: %v", count, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE payload->>'user_id'=$1 AND payload->>'entry_type'='opening_balance'", fmt.Sprint(uid)).Scan(&count); err != nil || count != 1 {
		t.Fatalf("starting outbox event count %d: %v", count, err)
	}
	in.OpeningMinor = 10
	if _, err := svc.Create(ctx, in); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatalf("changed account replay accepted: %v", err)
	}
	in.Name, in.IdempotencyKey = "Invalid future", "future"
	in.OpeningAt = time.Now().Add(24 * time.Hour)
	if _, err := svc.Create(ctx, in); !errors.Is(err, service.ErrOpeningBalance) {
		t.Fatalf("future starting balance accepted: %v", err)
	}
	// Fail after account insertion by blocking creation of the external account.
	other, _ := newUserWithAccount(t, "USD")
	if _, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: other, Name: "External", Currency: "USD"}); err != nil {
		t.Fatal(err)
	}
	in.UserID, in.Name, in.IdempotencyKey, in.OpeningAt = other, "Must roll back", "rollback", time.Now().Add(-time.Hour)
	if _, err := svc.Create(ctx, in); err == nil {
		t.Fatal("expected opening ledger write to fail")
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE user_id=$1 AND name=$2", other, in.Name).Scan(&count); err != nil || count != 0 {
		t.Fatalf("account survived failed opening ledger: %d %v", count, err)
	}
}

func TestWorkspaceActivityFiltersPaginationAndOwnership(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	otherUID, foreign := newUserWithAccount(t, "USD")
	date := time.Date(2024, 2, 29, 23, 59, 59, 999999000, time.UTC)
	var wanted int64
	for i := range 35 {
		note := fmt.Sprintf("Purchase %d", i)
		if i == 0 {
			note = "needle 100% _ grocery"
		}
		created, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: -100, Currency: "USD", OccurredAt: date, Note: note})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			wanted = created.ID
		}
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: otherUID, AccountID: foreign, AmountMinor: -100, Currency: "USD", OccurredAt: date, Note: "needle foreign"}); err != nil {
		t.Fatal(err)
	}
	target, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "Savings", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := txSvc.Transfer(ctx, service.TransferInput{UserID: uid, FromAccountID: account, ToAccountID: target.ID, AmountMinor: 99, Currency: "USD", OccurredAt: date})
	if err != nil {
		t.Fatal(err)
	}
	router := spendingRouter(uid)
	var page struct {
		Entries []struct {
			ID           int64 `json:"transaction_id"`
			Account      int64 `json:"account_id"`
			Counterparty int64 `json:"counterparty_account_id"`
			Transfer     bool  `json:"is_transfer"`
		} `json:"entries"`
		Next string `json:"next_cursor"`
	}
	seen := map[int64]bool{}
	cursor := ""
	for {
		getWorkspace(t, router, "/activity?limit=7&cursor="+cursor, &page)
		for _, row := range page.Entries {
			if seen[row.ID] {
				t.Fatal("duplicate transaction in paginated activity")
			}
			seen[row.ID] = true
		}
		cursor = page.Next
		if cursor == "" {
			break
		}
	}
	if len(seen) != 36 || !seen[transfer.ID] {
		t.Fatalf("lost entries or duplicated transfer: %d", len(seen))
	}
	getWorkspace(t, router, "/activity?search=needle&type=expense&start_date=2024-02-29&end_date=2024-02-29", &page)
	if len(page.Entries) != 1 || page.Entries[0].ID != wanted {
		t.Fatalf("search missed older page or leaked owner: %+v", page)
	}
	getWorkspace(t, router, fmt.Sprintf("/activity?account_id=%d&type=transfer", target.ID), &page)
	if len(page.Entries) != 1 || page.Entries[0].ID != transfer.ID || page.Entries[0].Account != account || page.Entries[0].Counterparty != target.ID || !page.Entries[0].Transfer {
		t.Fatalf("destination filter lost transfer context: %+v", page)
	}
	for _, path := range []string{"/activity?limit=-1", "/activity?limit=101", "/activity?cursor=bad", "/activity?type=bad", "/activity?category_id=-2", "/activity?start_date=2024-02-29", "/spending?type=all&currency=bad"} {
		r := httptest.NewRecorder()
		router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		if r.Code != http.StatusBadRequest {
			t.Fatalf("%s returned %d: %s", path, r.Code, r.Body.String())
		}
	}
	r := httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/activity?account_id=%d", foreign), nil))
	if r.Code != http.StatusNotFound {
		t.Fatalf("foreign account filter returned %d", r.Code)
	}
	for _, body := range []string{`{"entry_type":"opening_balance","amount_minor":1}`, `{"entry_type":"refund","amount_minor":-1}`} {
		var payload map[string]any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatal(err)
		}
		payload["account_id"], payload["currency"], payload["occurred_at"] = account, "USD", date
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRecorder()
		router.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/transactions", strings.NewReader(string(encoded))))
		if r.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid type accepted: %d %s", r.Code, r.Body.String())
		}
	}
}
