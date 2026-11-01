package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/money"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
)

func TestCategoryHintsOnlyOfferConfirmableTypes(t *testing.T) {
	uid, _ := newUserWithAccount(t, "USD")
	income := managedCategory(t, uid, 0, "Pay", "income")
	child := managedCategory(t, uid, income.ID, "Bonus", "income")
	expense := managedCategory(t, uid, 0, "Food", "expense")
	for _, tc := range []struct {
		note, kind string
		want       int64
	}{
		{"Bonus payment", "expense", 0},
		{"Bonus payment", "income", child.ID},
		{"Food refund", "refund", expense.ID},
		{"Food salary", "income", 0},
	} {
		body, err := json.Marshal(map[string]string{"note": tc.note, "kind": tc.kind, "currency": "USD"})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		adviceRouter(uid).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/categories/suggest", strings.NewReader(string(body))))
		var hint struct {
			ID int64 `json:"category_id"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &hint); err != nil || response.Code != 200 || hint.ID != tc.want {
			t.Fatalf("wrong hint for %s %s: %s %v", tc.note, tc.kind, response.Body.String(), err)
		}
	}
}

func TestDirectWritesProtectExactBalancesAndRetryClaims(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	at := time.Now().UTC().Add(-time.Second)
	in := service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", OccurredAt: at, AmountMinor: money.MaxSafeMinor}
	if _, err := txSvc.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.AmountMinor = money.MaxSafeMinor + 1
	if _, err := txSvc.Create(ctx, in); !errors.Is(err, service.ErrMoneyRange) {
		t.Fatal("unsafe individual amount accepted", err)
	}
	in.AmountMinor, in.IdempotencyKey = 1, "review-overflow"
	if _, err := txSvc.Create(ctx, in); !errors.Is(err, service.ErrMoneyRange) {
		t.Fatal("unsafe resulting balance accepted", err)
	}
	other, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "Transfer wallet", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txSvc.Transfer(ctx, service.TransferInput{UserID: uid, FromAccountID: other.ID, ToAccountID: account, AmountMinor: 1, Currency: "USD", OccurredAt: at}); !errors.Is(err, service.ErrMoneyRange) {
		t.Fatal("transfer overflow accepted", err)
	}
	otherBalance, err := balSvc.CurrentBalance(ctx, other.ID, time.Now())
	if err != nil || otherBalance.Balance != 0 {
		t.Fatal("failed transfer changed its source", otherBalance, err)
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", OccurredAt: at, AmountMinor: -1}); err != nil {
		t.Fatal(err)
	}
	first, err := txSvc.Create(ctx, in)
	if err != nil {
		t.Fatal("failed write retained its retry claim", err)
	}
	second, err := txSvc.Create(ctx, in)
	if err != nil || first.ID != second.ID {
		t.Fatal("successful retry was not idempotent", first.ID, second.ID, err)
	}
	final, err := balSvc.CurrentBalance(ctx, account, time.Now())
	if err != nil || final.Balance != money.MaxSafeMinor {
		t.Fatal("exact boundary changed", final, err)
	}
}

func TestStandaloneReversalProtectsExactBalanceAndRollsBack(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	in := service.CreateInput{UserID: uid, AccountID: account, Currency: "USD", OccurredAt: time.Now().UTC().Add(-time.Second), AmountMinor: money.MaxSafeMinor}
	if _, err := txSvc.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.AmountMinor = -1
	expense, err := txSvc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	in.AmountMinor = 1
	if _, err := txSvc.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	reverse := service.ReverseInput{UserID: uid, TransactionID: expense.ID, IdempotencyKey: "review-reverse-overflow"}
	if _, err := txSvc.Reverse(ctx, reverse); !errors.Is(err, service.ErrMoneyRange) {
		t.Fatal("unsafe reversal accepted", err)
	}
	var reversed int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM transactions WHERE reversal_of=$1", expense.ID).Scan(&reversed); err != nil || reversed != 0 {
		t.Fatal("failed reversal was partially committed", reversed, err)
	}
	in.AmountMinor = -1
	if _, err := txSvc.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	first, err := txSvc.Reverse(ctx, reverse)
	if err != nil {
		t.Fatal("failed reversal consumed retry key", err)
	}
	second, err := txSvc.Reverse(ctx, reverse)
	if err != nil || first.ID != second.ID {
		t.Fatal("reversal retry was not idempotent", err)
	}
	got, err := balSvc.CurrentBalance(ctx, account, time.Now())
	if err != nil || got.Balance != money.MaxSafeMinor {
		t.Fatal("reversal changed exact balance", got, err)
	}
}

func TestDirectWritesPreserveBackdatedHistoryAndRejectFutureTransfers(t *testing.T) {
	ctx := context.Background()
	uid, other := newUserWithAccount(t, "USD")
	start := time.Now().UTC().AddDate(0, -1, 0).Truncate(time.Second)
	a, err := service.NewAccountService(pool, q).Create(ctx, service.AccountInput{UserID: uid, Name: "Started wallet", Currency: "USD", OpeningMinor: 100, OpeningAt: start})
	if err != nil {
		t.Fatal(err)
	}
	before := start.Add(-time.Hour)
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: a.ID, AmountMinor: -1, Currency: "USD", OccurredAt: before}); err != nil {
		t.Fatal("existing manual-history workflow was rejected", err)
	}
	transfer := service.TransferInput{UserID: uid, FromAccountID: a.ID, ToAccountID: other, AmountMinor: 1, Currency: "USD", OccurredAt: before}
	if _, err := txSvc.Transfer(ctx, transfer); err != nil {
		t.Fatal("historical transfer was rejected", err)
	}
	transfer.FromAccountID, transfer.ToAccountID = other, a.ID
	if _, err := txSvc.Transfer(ctx, transfer); err != nil {
		t.Fatal("historical destination transfer was rejected", err)
	}
	transfer.OccurredAt = time.Now().UTC().AddDate(0, 1, 0)
	if _, err := txSvc.Transfer(ctx, transfer); !errors.Is(err, service.ErrFutureTransaction) {
		t.Fatal("future transfer was posted", err)
	}
	external, err := q.GetExternalAccount(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: external.ID, AmountMinor: 1, Currency: "USD", OccurredAt: time.Now().UTC().Add(-time.Second)}); !errors.Is(err, service.ErrAccountNotFound) {
		t.Fatal("internal offset account was accepted as a wallet", err)
	}
	transfer.OccurredAt = start.Add(time.Hour)
	if _, err := txSvc.Transfer(ctx, transfer); err != nil {
		t.Fatal("valid transfer rejected", err)
	}
	got, err := balSvc.CurrentBalance(ctx, a.ID, time.Now())
	if err != nil || got.Balance != 100 {
		t.Fatal("rejected writes changed recorded money", got, err)
	}
}

func TestCategoryConfirmationRespectsEntryTypeAndInheritance(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	income := managedCategory(t, uid, 0, "Pay", "income")
	child, err := service.NewCategoryService(pool, q).Save(ctx, uid, 0, service.CategoryEdit{Name: ptr("Bonus"), ParentID: ptr(income.ID)})
	if err != nil {
		t.Fatal(err)
	}
	expense := managedCategory(t, uid, 0, "Food", "expense")
	for i, tc := range []struct {
		amount             int64
		entryType          string
		allowed, forbidden int64
	}{
		{100, "standard", child.ID, expense.ID},
		{-25, "standard", expense.ID, child.ID},
		{10, "refund", expense.ID, income.ID},
	} {
		entry, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: tc.amount, EntryType: tc.entryType, Currency: "USD", OccurredAt: time.Now().UTC().Add(-time.Second), Note: fmt.Sprintf("type check %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if err := txSvc.ConfirmCategory(ctx, uid, entry.ID, tc.forbidden); !errors.Is(err, service.ErrInvalidCategory) {
			t.Fatal("mismatched type accepted", err)
		}
		if err := txSvc.ConfirmCategory(ctx, uid, entry.ID, tc.allowed); err != nil {
			t.Fatal("compatible type rejected", err)
		}
		stored, err := q.GetTransactionForUser(ctx, sqlc.GetTransactionForUserParams{ID: entry.ID, UserID: uid})
		if err != nil || stored.CategoryID.Int64 != tc.allowed {
			t.Fatal("confirmation did not persist", stored, err)
		}
	}
	balance, err := balSvc.CurrentBalance(ctx, account, time.Now())
	if err != nil || balance.Balance != 85 {
		t.Fatal("classification changed money", balance, err)
	}
}
