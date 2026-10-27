package integration

import (
	"context"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/ai"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/jackc/pgx/v5/pgtype"
)

// runCategorize replicates the worker's categorize handler against the real DB
// using the deterministic keyword categorizer — no API key or Redis needed.
// (The worker's handleCategorizeTx is unexported; this mirrors its logic so the
// suggest-then-confirm DB flow is covered by an integration test.)
func runCategorize(t *testing.T, c ai.Categorizer, userID, txID int64) {
	t.Helper()
	ctx := context.Background()
	tx, err := q.GetTransactionForUser(ctx, sqlc.GetTransactionForUserParams{ID: txID, UserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	if tx.CategoryID.Valid || !tx.Note.Valid || tx.Note.String == "" {
		return
	}
	cats, err := q.ListCategoriesByUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	candidates := make([]ai.Category, 0, len(cats))
	for _, cat := range cats {
		candidates = append(candidates, ai.Category{ID: cat.ID, Name: cat.Name})
	}
	amount, err := q.GetTransactionNormalAmount(ctx, txID)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Categorize(ctx, ai.CategorizeInput{
		Note: tx.Note.String, Currency: tx.Currency, AmountMinor: amount, Candidates: candidates,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CategoryID == 0 {
		return
	}
	if err := q.SetCategorySuggestion(ctx, sqlc.SetCategorySuggestionParams{
		ID:                  txID,
		SuggestedCategoryID: pgtype.Int8{Int64: res.CategoryID, Valid: true},
		SuggestionScore:     pgtype.Float8{Float64: res.Confidence, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCategorizeSuggestThenConfirm(t *testing.T) {
	ctx := context.Background()
	uid, accID := newUserWithAccount(t, "USD")

	groceries, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: uid, Name: "Groceries"})
	if err != nil {
		t.Fatal(err)
	}

	created, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accID, AmountMinor: -4200, Currency: "USD",
		OccurredAt: time.Now().UTC(), Note: "weekly groceries run",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1) AI proposes a suggestion — category_id stays NULL.
	runCategorize(t, ai.KeywordCategorizer{}, uid, created.ID)
	row, err := q.GetTransactionForUser(ctx, sqlc.GetTransactionForUserParams{ID: created.ID, UserID: uid})
	if err != nil {
		t.Fatal(err)
	}
	if row.CategoryID.Valid {
		t.Fatal("AI must NOT set the confirmed category_id")
	}
	if !row.SuggestedCategoryID.Valid || row.SuggestedCategoryID.Int64 != groceries.ID {
		t.Fatalf("expected suggestion of category %d, got %+v", groceries.ID, row.SuggestedCategoryID)
	}

	// 2) Human confirms — suggestion is promoted and cleared.
	confirmed, err := q.ConfirmTransactionCategory(ctx, sqlc.ConfirmTransactionCategoryParams{
		ID: created.ID, UserID: uid, CategoryID: pgtype.Int8{Int64: groceries.ID, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed.CategoryID.Valid || confirmed.CategoryID.Int64 != groceries.ID {
		t.Fatalf("confirmed category = %+v, want %d", confirmed.CategoryID, groceries.ID)
	}

	// 3) A late re-run must NOT clobber the confirmed category.
	runCategorize(t, ai.KeywordCategorizer{}, uid, created.ID)
	after, err := q.GetTransactionForUser(ctx, sqlc.GetTransactionForUserParams{ID: created.ID, UserID: uid})
	if err != nil {
		t.Fatal(err)
	}
	if after.SuggestedCategoryID.Valid {
		t.Fatal("re-categorization must be a no-op once confirmed")
	}
	if !after.CategoryID.Valid || after.CategoryID.Int64 != groceries.ID {
		t.Fatal("confirmed category must survive a late categorize attempt")
	}
}
