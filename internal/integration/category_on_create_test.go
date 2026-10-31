package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
)

func TestCategorySavedAtomicallyWithIncomeAndExpense(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	otherUID, _ := newUserWithAccount(t, "USD")
	category, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: uid, Name: "Selected category"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: otherUID, Name: "Foreign category"})
	if err != nil {
		t.Fatal(err)
	}
	input := service.CreateInput{UserID: uid, AccountID: account, AmountMinor: 100, Currency: "USD", OccurredAt: time.Now().UTC(), CategoryID: category.ID, IdempotencyKey: "category-on-create"}
	created, err := txSvc.Create(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if created.CategoryID != category.ID {
		t.Fatalf("response omitted category: %+v", created)
	}
	replayed, err := txSvc.Create(ctx, input)
	if err != nil || replayed.ID != created.ID || replayed.CategoryID != category.ID {
		t.Fatalf("replay changed categorized transaction: %+v %v", replayed, err)
	}
	changed := input
	changed.CategoryID = 0
	if _, err := txSvc.Create(ctx, changed); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatalf("category change must change request identity: %v", err)
	}
	input.AmountMinor = -25
	input.IdempotencyKey = "expense-category"
	expense, err := txSvc.Create(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{created.ID, expense.ID} {
		header, err := q.GetTransactionForUser(ctx, sqlc.GetTransactionForUserParams{ID: id, UserID: uid})
		if err != nil {
			t.Fatal(err)
		}
		if !header.CategoryID.Valid || header.CategoryID.Int64 != category.ID || header.SuggestedCategoryID.Valid {
			t.Fatalf("chosen category was not authoritative: %+v", header)
		}
	}
	input.IdempotencyKey = "foreign-category-must-roll-back"
	input.CategoryID = foreign.ID
	if _, err := txSvc.Create(ctx, input); !errors.Is(err, service.ErrInvalidCategory) {
		t.Fatalf("accepted foreign category: %v", err)
	}
	// A failed category check must roll back the idempotency claim and postings.
	input.CategoryID = category.ID
	if _, err := txSvc.Create(ctx, input); err != nil {
		t.Fatalf("failed category left an idempotency claim: %v", err)
	}
	balance, err := balSvc.CurrentBalance(ctx, account, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if balance.Balance != 50 {
		t.Fatalf("foreign category wrote an extra posting: balance %d", balance.Balance)
	}
}
