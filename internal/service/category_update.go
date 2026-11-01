package service

import (
	"context"
	"errors"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ConfirmCategory serializes category changes with correction previews/commits.
func (s *TransactionService) ConfirmCategory(ctx context.Context, uid, id, category int64) error {
	if category <= 0 {
		return ErrInvalidCategory
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return err
	}
	if _, err := selectableCategory(ctx, q, uid, category); err != nil {
		return ErrInvalidCategory
	}
	row, err := q.GetTransactionForUserForUpdate(ctx, sqlc.GetTransactionForUserForUpdateParams{ID: id, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTxNotFound
	}
	if err != nil {
		return err
	}
	if row.ReversalOf.Valid {
		return ErrIsReversal
	}
	origin, err := q.GetCorrectionOrigin(ctx, sqlc.GetCorrectionOriginParams{ID: id, UserID: uid})
	if err != nil {
		return err
	}
	if origin.ReversedBy != 0 {
		return ErrAlreadyReversed
	}
	amount, err := q.GetTransactionNormalAmount(ctx, id)
	if err != nil {
		return err
	}
	if err := validateEntryCategory(ctx, q, uid, category, amount, row.EntryType); err != nil {
		return err
	}
	if _, err := q.ConfirmTransactionCategory(ctx, sqlc.ConfirmTransactionCategoryParams{ID: id, UserID: uid, CategoryID: pgtype.Int8{Int64: category, Valid: true}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
