package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrPlanInput identifies an invalid planned entry.
	ErrPlanInput = errors.New("check the amount, account, category, and future date")
	// ErrPlanNotFound hides absent or foreign plans.
	ErrPlanNotFound = errors.New("planned activity not found")
	// ErrPlanClosed rejects a different action on a resolved plan.
	ErrPlanClosed = errors.New("this plan is no longer pending")
)

// PlanService keeps commitments outside the posted ledger until confirmation.
type PlanService struct {
	pool    *pgxpool.Pool
	q       *sqlc.Queries
	summary *SummaryService
}

// NewPlanService wires manual confirmation to the balanced ledger.
func NewPlanService(pool *pgxpool.Pool, q *sqlc.Queries, summary *SummaryService) *PlanService {
	return &PlanService{pool, q, summary}
}

// PlanInput describes future money without creating a posting.
type PlanInput struct {
	AccountID     int64     `json:"account_id"`
	AmountMinor   int64     `json:"amount_minor"`
	Currency      string    `json:"currency"`
	CategoryID    int64     `json:"category_id"`
	OccurredAt    time.Time `json:"occurred_at"`
	Note          string    `json:"note"`
	EntryType     string    `json:"entry_type"`
	DateOnly      bool      `json:"date_only"`
	ExcludeReport bool      `json:"exclude_from_reports"`
}

// Create saves a future plan once for a stable request key.
func (s *PlanService) Create(ctx context.Context, uid int64, key string, in PlanInput) (sqlc.PlannedTransaction, error) {
	var empty sqlc.PlannedTransaction
	if in.AccountID <= 0 || in.AmountMinor == 0 || !safeMinor(in.AmountMinor) || in.OccurredAt.IsZero() || in.OccurredAt.Year() < 1 || in.OccurredAt.Year() > 9999 || len([]rune(in.Note)) > 1000 || len(key) > 128 || key == "" {
		return empty, ErrPlanInput
	}
	if in.DateOnly && (in.OccurredAt.UTC().Hour() != 0 || in.OccurredAt.UTC().Minute() != 0 || in.OccurredAt.UTC().Second() != 0 || in.OccurredAt.UTC().Nanosecond() != 0) {
		return empty, ErrPlanInput
	}
	if in.EntryType == "" {
		in.EntryType = "standard"
	}
	if in.EntryType != "standard" && in.EntryType != "refund" || in.EntryType == "refund" && in.AmountMinor < 0 {
		return empty, ErrPlanInput
	}
	in.Note = strings.TrimSpace(in.Note)
	hash := importing.Hash(in)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return empty, err
	}
	previous, err := q.GetPlanByRequest(ctx, sqlc.GetPlanByRequestParams{UserID: uid, CreationKey: pgtype.Text{String: key, Valid: true}})
	if err == nil {
		if previous.CreationHash.String != hash {
			return empty, ErrIdempotencyConflict
		}
		return previous, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	if !in.OccurredAt.After(time.Now().UTC()) {
		return empty, ErrPlanInput
	}
	a, err := loadLedgerContext(ctx, q, uid, in.AccountID)
	if err != nil {
		return empty, err
	}
	if !a.Active {
		return empty, ErrAccountInactive
	}
	if a.Currency != in.Currency {
		return empty, ErrCurrencyMismatch
	}
	if in.CategoryID != 0 {
		if _, err := q.GetCategoryForUser(ctx, sqlc.GetCategoryForUserParams{ID: in.CategoryID, UserID: uid}); err != nil {
			return empty, ErrInvalidCategory
		}
	}
	plan, err := q.CreatePlan(ctx, sqlc.CreatePlanParams{UserID: uid, AccountID: in.AccountID, Currency: in.Currency, AmountMinor: in.AmountMinor, CategoryID: pgtype.Int8{Int64: in.CategoryID, Valid: in.CategoryID != 0}, OccurredAt: in.OccurredAt, Note: in.Note, EntryType: in.EntryType, DateOnly: in.DateOnly, ExcludeFromReports: in.ExcludeReport, CreationKey: pgtype.Text{String: key, Valid: true}, CreationHash: pgtype.Text{String: hash, Valid: true}})
	if err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return plan, nil
}

// Resolve is serialized with imports/corrections/deletion. Retrying a posted plan
// returns the original transaction identity and cannot create another expense.
func (s *PlanService) Resolve(ctx context.Context, uid, id int64, date string, cancel bool) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return 0, err
	}
	p, err := q.GetPlanForUser(ctx, sqlc.GetPlanForUserParams{ID: id, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrPlanNotFound
	}
	if err != nil {
		return 0, err
	}
	if p.Status == "posted" && !cancel {
		return p.TransactionID.Int64, nil
	}
	if p.Status == "canceled" && cancel {
		return 0, nil
	}
	if p.Status != "pending" {
		return 0, ErrPlanClosed
	}
	if cancel {
		if err := q.CancelPlan(ctx, sqlc.CancelPlanParams{ID: id, UserID: uid}); err != nil {
			return 0, err
		}
		return 0, tx.Commit(ctx)
	}
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	at, _, err := importing.Date(date, "iso")
	if err != nil || at.After(time.Now().UTC()) {
		return 0, ErrPlanInput
	}
	a, err := loadLedgerContext(ctx, q, uid, p.AccountID)
	if err != nil {
		return 0, err
	}
	if !a.Active {
		return 0, ErrAccountInactive
	}
	if _, err := addMinor(a.BalanceMinor, p.AmountMinor); err != nil {
		return 0, err
	}
	external, err := q.EnsureExternalAccount(ctx, sqlc.EnsureExternalAccountParams{UserID: uid, Currency: p.Currency})
	if err != nil {
		return 0, err
	}
	dto, err := appendLedger(ctx, q, ledgerWrite{UserID: uid, Currency: p.Currency, At: at, Note: p.Note, CategoryID: p.CategoryID.Int64, EntryType: p.EntryType, ExcludeReport: p.ExcludeFromReports, ImportID: p.ImportID.Int64, Legs: []Leg{{AccountID: p.AccountID, AmountMinor: p.AmountMinor}, {AccountID: external, AmountMinor: -p.AmountMinor}}})
	if err != nil {
		return 0, err
	}
	if p.ImportID.Valid {
		if err := q.CreateImportedTransaction(ctx, sqlc.CreateImportedTransactionParams{UserID: uid, AccountID: p.AccountID, ImportID: p.ImportID.Int64, TransactionID: dto.ID, RowNumber: p.RowNumber.Int32, Fingerprint: p.Fingerprint, SourceRef: p.SourceRef, DateOnly: true}); err != nil {
			return 0, err
		}
	}
	if err := q.CompletePlan(ctx, sqlc.CompletePlanParams{ID: id, UserID: uid, TransactionID: pgtype.Int8{Int64: dto.ID, Valid: true}}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	if s.summary != nil {
		s.summary.Invalidate(ctx, p.AccountID, at)
		s.summary.Invalidate(ctx, external, at)
	}
	return dto.ID, nil
}
