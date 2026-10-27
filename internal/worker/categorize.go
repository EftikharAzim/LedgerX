// Package worker processes queued exports, snapshots, categorization, and outbox events.
package worker

import (
	"context"
	"encoding/json"

	"github.com/EftikharAzim/ledgerx/internal/ai"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// handleCategorizeTx asks the Categorizer to propose a category and stores it
// as a suggestion. It never sets category_id — confirmation is a human action.
// The SetCategorySuggestion query is a no-op if the transaction has since been
// categorized, so a late job can't clobber a confirmation.
func (s *Server) handleCategorizeTx(ctx context.Context, t *asynq.Task) error {
	if s.categorizer == nil {
		return nil
	}
	var p CategorizeTxPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}

	tx, err := s.q.GetTransactionForUser(ctx, sqlc.GetTransactionForUserParams{
		ID: p.TransactionID, UserID: p.UserID,
	})
	if err != nil {
		return err
	}
	// Skip if there's nothing to classify or it's already categorized.
	if tx.CategoryID.Valid || !tx.Note.Valid || tx.Note.String == "" {
		return nil
	}

	cats, err := s.q.ListCategoriesByUser(ctx, p.UserID)
	if err != nil {
		return err
	}
	if len(cats) == 0 {
		return nil // user has no categories to choose from
	}
	candidates := make([]ai.Category, 0, len(cats))
	for _, c := range cats {
		candidates = append(candidates, ai.Category{ID: c.ID, Name: c.Name})
	}

	amount, err := s.q.GetTransactionNormalAmount(ctx, tx.ID)
	if err != nil {
		return err
	}

	res, err := s.categorizer.Categorize(ctx, ai.CategorizeInput{
		Note:        tx.Note.String,
		Currency:    tx.Currency,
		AmountMinor: amount,
		Candidates:  candidates,
	})
	if err != nil {
		return err // asynq retries transient model/API failures
	}
	if res.CategoryID == 0 {
		return nil // model declined to guess
	}

	return s.q.SetCategorySuggestion(ctx, sqlc.SetCategorySuggestionParams{
		ID:                  tx.ID,
		SuggestedCategoryID: pgtype.Int8{Int64: res.CategoryID, Valid: true},
		SuggestionScore:     pgtype.Float8{Float64: res.Confidence, Valid: true},
	})
}
