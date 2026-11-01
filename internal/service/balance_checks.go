package service

import (
	"context"
	"errors"
	"strings"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BalanceCheckService records comparisons; it never adjusts a ledger balance.
type BalanceCheckService struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewBalanceCheckService uses the same consistent account state as imports.
func NewBalanceCheckService(pool *pgxpool.Pool, q *sqlc.Queries) *BalanceCheckService {
	return &BalanceCheckService{pool: pool, q: q}
}

// BalanceCheckInput is the actual posted balance observed by the user.
type BalanceCheckInput struct {
	UserID, AccountID, ActualMinor int64
	Note, Key                      string
}

// Check stores a snapshot of the comparison and supports retry-safe submission.
func (s *BalanceCheckService) Check(ctx context.Context, in BalanceCheckInput) (sqlc.BalanceCheck, error) {
	var empty sqlc.BalanceCheck
	in.Note = strings.TrimSpace(in.Note)
	if !safeMinor(in.ActualMinor) {
		return empty, ErrMoneyRange
	}
	if len([]rune(in.Note)) > 200 || len(in.Key) > 128 {
		return empty, ErrImportSelection
	}
	hash := importing.Hash(struct {
		Account, Actual int64
		Note            string
	}{in.AccountID, in.ActualMinor, in.Note})
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, in.UserID); err != nil {
		return empty, err
	}
	if in.Key != "" {
		previous, err := q.GetBalanceCheckByKey(ctx, sqlc.GetBalanceCheckByKeyParams{UserID: in.UserID, RequestKey: pgtype.Text{String: in.Key, Valid: true}})
		if err == nil {
			if previous.RequestHash.String != hash {
				return empty, ErrIdempotencyConflict
			}
			return previous, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return empty, err
		}
	}
	account, err := loadLedgerContext(ctx, q, in.UserID, in.AccountID)
	if err != nil {
		return empty, err
	}
	if !account.Active {
		return empty, ErrAccountInactive
	}
	difference, err := addMinor(in.ActualMinor, -account.BalanceMinor)
	if err != nil {
		return empty, err
	}
	check, err := q.CreateBalanceCheck(ctx, sqlc.CreateBalanceCheckParams{UserID: in.UserID, AccountID: in.AccountID, ActualMinor: in.ActualMinor, RecordedMinor: account.BalanceMinor, DifferenceMinor: difference, LedgerRevision: account.Revision, Note: in.Note, RequestKey: pgtype.Text{String: in.Key, Valid: in.Key != ""}, RequestHash: pgtype.Text{String: hash, Valid: in.Key != ""}})
	if err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return check, nil
}

// History returns recent checks alongside the current account revision.
func (s *BalanceCheckService) History(ctx context.Context, uid, accountID int64) ([]sqlc.BalanceCheck, AccountLedgerContext, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, AccountLedgerContext{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	account, err := loadLedgerContext(ctx, q, uid, accountID)
	if err != nil {
		return nil, account, err
	}
	checks, err := q.ListBalanceChecks(ctx, sqlc.ListBalanceChecksParams{UserID: uid, AccountID: accountID})
	if err != nil {
		return nil, account, err
	}
	return checks, account, tx.Commit(ctx)
}
