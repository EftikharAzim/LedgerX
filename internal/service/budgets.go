package service

import (
	"context"
	"errors"
	"strings"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/reporting"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/currency"
)

// ErrBudgetInput describes an invalid monthly limit or scope.
var ErrBudgetInput = errors.New("choose a valid month, currency, spending category and positive exact limit; at most 100 budgets per month")

// ErrBudgetNotFound hides missing and foreign budgets.
var ErrBudgetNotFound = errors.New("budget not found")

// ErrBudgetConflict requires resolving an existing scope before moving a budget.
var ErrBudgetConflict = errors.New("a budget already exists for this month, currency and category")

// BudgetService derives usage from the corrected posted ledger, never plans.
type BudgetService struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewBudgetService uses existing database and ownership locks.
func NewBudgetService(pool *pgxpool.Pool, q *sqlc.Queries) *BudgetService {
	return &BudgetService{pool, q}
}

// BudgetInput identifies one monthly envelope; zero category means overall.
type BudgetInput struct {
	ID          int64  `json:"id"`
	Month       string `json:"month"`
	Currency    string `json:"currency"`
	CategoryID  int64  `json:"category_id"`
	AmountMinor int64  `json:"amount_minor"`
}

// Budget contains the exact net spend and remaining limit for its scope.
type Budget struct {
	BudgetInput
	Name           string `json:"category_name"`
	ParentID       int64  `json:"parent_id"`
	Archived       bool   `json:"archived"`
	SpentMinor     int64  `json:"spent_minor"`
	RemainingMinor int64  `json:"remaining_minor"`
}

func budgetMonth(month, code string) (time.Time, error) {
	p, err := reporting.Parse(reporting.Selection{Type: "monthly", Month: month})
	_, ce := currency.ParseISO(code)
	if err != nil || ce != nil || len(code) != 3 || code != strings.ToUpper(code) {
		return time.Time{}, ErrBudgetInput
	}
	return p.Start, nil
}

// List includes category children exactly once. Overall and child budgets are
// independent views of the same money and must not be added together.
func (s *BudgetService) List(ctx context.Context, uid int64, month, code string) ([]Budget, error) {
	at, err := budgetMonth(month, code)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT b.id,b.category_id,b.amount_minor,COALESCE(c.name,'Overall monthly budget'),COALESCE(c.parent_id,0),COALESCE(c.archived,false),
 COALESCE((SELECT sum(CASE WHEN t.entry_type='refund' THEN -p.amount_minor WHEN p.amount_minor<0 THEN -p.amount_minor ELSE 0 END)
 FROM transactions t JOIN postings p ON p.transaction_id=t.id JOIN accounts a ON a.id=p.account_id AND a.kind='normal'
 LEFT JOIN categories child ON child.id=t.category_id
 WHERE t.user_id=b.user_id AND t.currency=b.currency AND t.occurred_at>=b.month::timestamp AT TIME ZONE 'UTC'
 AND t.occurred_at<(b.month+INTERVAL '1 month')::timestamp AT TIME ZONE 'UTC'
 AND t.entry_type<>'opening_balance' AND NOT t.exclude_from_reports AND t.reversal_of IS NULL
 AND NOT EXISTS(SELECT 1 FROM transactions r WHERE r.reversal_of=t.id)
 AND EXISTS(SELECT 1 FROM postings ep JOIN accounts ea ON ea.id=ep.account_id WHERE ep.transaction_id=t.id AND ea.kind='external')
 AND (b.category_id IS NULL OR t.category_id=b.category_id OR child.parent_id=b.category_id)),0)::text
 FROM budgets b LEFT JOIN categories c ON c.id=b.category_id WHERE b.user_id=$1 AND b.currency=$2 AND b.month=$3 ORDER BY b.category_id IS NOT NULL,COALESCE(c.name,''),b.id`, uid, code, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Budget{}
	for rows.Next() {
		b := Budget{BudgetInput: BudgetInput{Month: month, Currency: code}}
		var cat pgtype.Int8
		var spent string
		if err := rows.Scan(&b.ID, &cat, &b.AmountMinor, &b.Name, &b.ParentID, &b.Archived, &spent); err != nil {
			return nil, err
		}
		b.CategoryID = cat.Int64
		b.SpentMinor, err = parseExactAggregate(spent)
		if err != nil {
			return nil, err
		}
		b.RemainingMinor, err = addMinor(b.AmountMinor, -b.SpentMinor)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Save upserts one scope or edits a named budget. Retrying the same limit is harmless.
func (s *BudgetService) Save(ctx context.Context, uid int64, in BudgetInput) (int64, error) {
	at, err := budgetMonth(in.Month, in.Currency)
	if err != nil || in.AmountMinor <= 0 || !safeMinor(in.AmountMinor) || in.CategoryID < 0 || in.ID < 0 {
		return 0, ErrBudgetInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return 0, err
	}
	if in.CategoryID != 0 {
		c, err := q.GetCategoryForUser(ctx, sqlc.GetCategoryForUserParams{ID: in.CategoryID, UserID: uid})
		if err != nil || c.Deleted || c.Kind == "income" {
			return 0, ErrBudgetInput
		}
	}
	var count int
	var existing int64
	if err := tx.QueryRow(ctx, "SELECT count(*),COALESCE(max(id) FILTER(WHERE COALESCE(category_id,0)=$4),0) FROM budgets WHERE user_id=$1 AND currency=$2 AND month=$3", uid, in.Currency, at, in.CategoryID).Scan(&count, &existing); err != nil {
		return 0, err
	}
	if count >= 100 && existing == 0 {
		return 0, ErrBudgetInput
	}
	if in.ID != 0 {
		tag, err := tx.Exec(ctx, "UPDATE budgets SET currency=$3,month=$4,category_id=$5,amount_minor=$6,updated_at=now() WHERE user_id=$1 AND id=$2", uid, in.ID, in.Currency, at, pgtype.Int8{Int64: in.CategoryID, Valid: in.CategoryID != 0}, in.AmountMinor)
		if isUniqueViolation(err) {
			return 0, ErrBudgetConflict
		}
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() == 0 {
			return 0, ErrBudgetNotFound
		}
		return in.ID, tx.Commit(ctx)
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO budgets(user_id,currency,month,category_id,amount_minor) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(user_id,currency,month,(COALESCE(category_id,0))) DO UPDATE SET amount_minor=excluded.amount_minor,updated_at=now() RETURNING id`, uid, in.Currency, at, pgtype.Int8{Int64: in.CategoryID, Valid: in.CategoryID != 0}, in.AmountMinor).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

// Delete removes only a limit, leaving all financial activity intact.
func (s *BudgetService) Delete(ctx context.Context, uid, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.q.WithTx(tx).LockUserLedger(ctx, uid); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "DELETE FROM budgets WHERE user_id=$1 AND id=$2", uid, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBudgetNotFound
	}
	return tx.Commit(ctx)
}

// Copy reuses active category limits without replacing budgets already set.
func (s *BudgetService) Copy(ctx context.Context, uid int64, from, to, code string) (int64, error) {
	a, err := budgetMonth(from, code)
	if err != nil {
		return 0, err
	}
	b, err := budgetMonth(to, code)
	if err != nil || from == to {
		return 0, ErrBudgetInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.q.WithTx(tx).LockUserLedger(ctx, uid); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO budgets(user_id,currency,month,category_id,amount_minor)
 SELECT b.user_id,b.currency,$4,b.category_id,b.amount_minor FROM budgets b LEFT JOIN categories c ON c.id=b.category_id LEFT JOIN categories parent ON parent.id=c.parent_id
 WHERE b.user_id=$1 AND b.currency=$2 AND b.month=$3 AND (b.category_id IS NULL OR (NOT c.archived AND NOT c.deleted AND COALESCE(parent.archived,false)=false))
 ON CONFLICT DO NOTHING`, uid, code, a, b)
	if err != nil {
		return 0, err
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM budgets WHERE user_id=$1 AND currency=$2 AND month=$3", uid, code, b).Scan(&count); err != nil {
		return 0, err
	}
	if count > 100 {
		return 0, ErrBudgetInput
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}
