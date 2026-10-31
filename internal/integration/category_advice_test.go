package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func adviceRouter(uid int64) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), httptransport.UserIDKey, uid)))
		})
	})
	httptransport.NewCategoriesAPI(q).Routes(r)
	return r
}

func TestCategoryAdviceUsesOnlyConfirmedPersonalMatches(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	food, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: uid, Name: "Food"})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: uid, Name: "Transport"})
	if err != nil {
		t.Fatal(err)
	}
	record := func(owner, accountID, amount, category int64, currency, note string) service.TransactionDTO {
		t.Helper()
		tx, err := txSvc.Create(ctx, service.CreateInput{UserID: owner, AccountID: accountID, AmountMinor: amount, CategoryID: category, Currency: currency, Note: note, OccurredAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		return tx
	}
	first := record(uid, account, -100, food.ID, "USD", "Morning Coffee")
	record(uid, account, -100, food.ID, "USD", " morning\tcoffee ")
	record(uid, account, 100, transport.ID, "USD", "Morning Coffee")
	record(uid, account, -100, transport.ID, "USD", "Morning Coffee cart")
	canceled := record(uid, account, -100, transport.ID, "USD", "Morning Coffee")
	if _, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: canceled.ID}); err != nil {
		t.Fatal(err)
	}
	proposal := record(uid, account, -100, 0, "USD", "Morning Coffee")
	if err := q.SetCategorySuggestion(ctx, sqlc.SetCategorySuggestionParams{ID: proposal.ID, SuggestedCategoryID: pgtype.Int8{Int64: transport.ID, Valid: true}, SuggestionScore: pgtype.Float8{Float64: .9, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	jpy, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "Yen", Currency: "JPY"})
	if err != nil {
		t.Fatal(err)
	}
	record(uid, jpy.ID, -100, transport.ID, "JPY", "Morning Coffee")
	foreignUID, foreignAccount := newUserWithAccount(t, "USD")
	foreign, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: foreignUID, Name: "Private category"})
	if err != nil {
		t.Fatal(err)
	}
	record(foreignUID, foreignAccount, -100, foreign.ID, "USD", "Morning Coffee")
	target, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "Savings", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := txSvc.Transfer(ctx, service.TransferInput{UserID: uid, FromAccountID: account, ToAccountID: target.ID, AmountMinor: 50, Currency: "USD", OccurredAt: time.Now().UTC(), Note: "Morning Coffee"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.ConfirmTransactionCategory(ctx, sqlc.ConfirmTransactionCategoryParams{ID: transfer.ID, UserID: uid, CategoryID: pgtype.Int8{Int64: transport.ID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE user_id=$1", uid).Scan(&before); err != nil {
		t.Fatal(err)
	}
	router := adviceRouter(uid)
	check := func(note, currency, kind string) struct {
		ID      int64  `json:"category_id"`
		Source  string `json:"source"`
		Support int    `json:"supporting_records"`
		Samples int    `json:"matching_records"`
	} { t.Helper(); body, err := json.Marshal(map[string]string{"note": note, "currency": currency, "kind": kind}); if err != nil {
		t.Fatal(err)
	}; r := httptest.NewRecorder(); router.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/categories/suggest", strings.NewReader(string(body)))); if r.Code != http.StatusOK {
		t.Fatalf("suggestion failed: %d %s", r.Code, r.Body.String())
	}; if r.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private draft suggestion was cacheable")
	}; var result struct {
		ID      int64  `json:"category_id"`
		Source  string `json:"source"`
		Support int    `json:"supporting_records"`
		Samples int    `json:"matching_records"`
	}; if err := json.Unmarshal(r.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}; return result }
	if got := check(" MORNING  Coffee ", "USD", "expense"); got.ID != food.ID || got.Source != "history" || got.Support != 2 || got.Samples != 2 {
		t.Fatalf("invalid learning inputs leaked into advice: %+v", got)
	}
	if got := check("Morning Coffee", "USD", "income"); got.ID != transport.ID || got.Samples != 1 {
		t.Fatalf("income was mixed with expenses: %+v", got)
	}
	if got := check("Morning Coffee", "USD", "refund"); got.ID != 0 {
		t.Fatalf("a refund borrowed income/expense history: %+v", got)
	}
	if got := check("weekly Food", "USD", "expense"); got.ID != food.ID || got.Source != "keyword" {
		t.Fatalf("missing free fallback: %+v", got)
	}
	if got := check("Private category", "USD", "expense"); got.ID != 0 {
		t.Fatalf("another owner's category leaked: %+v", got)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE user_id=$1", uid).Scan(&after); err != nil || before != after {
		t.Fatalf("suggestion changed ledger count: %d/%d %v", before, after, err)
	}
	if _, err := q.ConfirmTransactionCategory(ctx, sqlc.ConfirmTransactionCategoryParams{ID: first.ID, UserID: uid, CategoryID: pgtype.Int8{Int64: transport.ID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if got := check("Morning Coffee", "USD", "expense"); got.ID != 0 || got.Source != "ambiguous" {
		t.Fatalf("conflicting personal choices should abstain: %+v", got)
	}
}

func TestCategoryAdviceValidatesDraftAndAuthentication(t *testing.T) {
	uid, _ := newUserWithAccount(t, "USD")
	router := adviceRouter(uid)
	for _, body := range []string{`{"note":"a","currency":"USD","kind":"expense"}`, `{"note":"Food","currency":"usd","kind":"expense"}`, `{"note":"Food","currency":"USD","kind":"transfer"}`, `{"note":"Food","currency":"USD","kind":"opening_balance"}`, strings.Repeat("x", 5000)} {
		r := httptest.NewRecorder()
		router.ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/categories/suggest", strings.NewReader(body)))
		if r.Code != http.StatusBadRequest {
			t.Fatalf("invalid draft accepted: %d", r.Code)
		}
	}
	r := httptest.NewRecorder()
	adviceRouter(0).ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/categories/suggest", strings.NewReader(`{}`)))
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated advice returned %d", r.Code)
	}
}
