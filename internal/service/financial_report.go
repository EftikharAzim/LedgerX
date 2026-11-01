package service

import (
	"context"
	"strconv"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/reporting"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func parseExactAggregate(s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || !safeMinor(v) {
		return 0, ErrMoneyRange
	}
	return v, nil
}

// ReportCategory holds both sides of a category without duplicating parent totals.
type ReportCategory struct {
	ID       int64  `json:"category_id"`
	ParentID int64  `json:"parent_id"`
	Name     string `json:"name"`
	Income   int64  `json:"income_minor"`
	Spending int64  `json:"spending_minor"`
	Count    int64  `json:"count"`
}

// ReportTotals separates posted balance movement from classified net income.
type ReportTotals struct {
	Income   int64 `json:"income_minor"`
	Spending int64 `json:"spending_minor"`
	Net      int64 `json:"net_minor"`
}

// ReportTrend represents a selectable calendar month.
type ReportTrend struct {
	Month string `json:"month"`
	ReportTotals
}

// FinancialReport is a single currency, coherent snapshot of recorded money.
type FinancialReport struct {
	Currency string `json:"currency"`
	Start    string `json:"start_date"`
	End      string `json:"end_date"`
	Opening  int64  `json:"opening_minor"`
	Ending   int64  `json:"ending_minor"`
	Other    int64  `json:"other_movement_minor"`
	ReportTotals
	Categories    []ReportCategory `json:"categories"`
	Previous      *ReportTotals    `json:"previous,omitempty"`
	Change        *ReportTotals    `json:"change,omitempty"`
	PreviousStart string           `json:"previous_start,omitempty"`
	PreviousEnd   string           `json:"previous_end,omitempty"`
	Trends        []ReportTrend    `json:"trends"`
}

// FinancialReportService reads reports from one repeatable snapshot.
type FinancialReportService struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewFinancialReportService shares the existing report and ledger rules.
func NewFinancialReportService(pool *pgxpool.Pool, q *sqlc.Queries) *FinancialReportService {
	return &FinancialReportService{pool, q}
}

func reportCategories(ctx context.Context, q *sqlc.Queries, uid int64, code string, p reporting.Period) ([]ReportCategory, ReportTotals, error) {
	rows, err := q.GetSpendingByCategory(ctx, sqlc.GetSpendingByCategoryParams{UserID: uid, Currency: code, StartDate: pgtype.Date{Time: p.Start, Valid: p.Type != "all"}, EndDate: pgtype.Date{Time: p.End, Valid: p.Type != "all"}})
	if err != nil {
		return nil, ReportTotals{}, err
	}
	cats, err := q.ListCategoriesByUser(ctx, uid)
	if err != nil {
		return nil, ReportTotals{}, err
	}
	parents := map[int64]int64{}
	for _, c := range cats {
		parents[c.ID] = c.ParentID.Int64
	}
	out := []ReportCategory{}
	total := ReportTotals{}
	groups := map[int64]ReportTotals{}
	for _, r := range rows {
		if !safeMinor(r.IncomeMinor) || !safeMinor(r.SpendingMinor) {
			return nil, total, ErrMoneyRange
		}
		id := r.CategoryID.Int64
		if parents[id] != 0 {
			id = parents[id]
		}
		g := groups[id]
		g.Income, err = addMinor(g.Income, r.IncomeMinor)
		if err != nil {
			return nil, total, err
		}
		g.Spending, err = addMinor(g.Spending, r.SpendingMinor)
		if err != nil {
			return nil, total, err
		}
		groups[id] = g
		total.Income, err = addMinor(total.Income, r.IncomeMinor)
		if err != nil {
			return nil, total, err
		}
		total.Spending, err = addMinor(total.Spending, r.SpendingMinor)
		if err != nil {
			return nil, total, err
		}
		out = append(out, ReportCategory{ID: r.CategoryID.Int64, ParentID: parents[r.CategoryID.Int64], Name: r.CategoryName, Income: r.IncomeMinor, Spending: r.SpendingMinor, Count: r.SpendingCount})
	}
	total.Net, err = addMinor(total.Income, -total.Spending)
	return out, total, err
}

func priorPeriod(p reporting.Period) (reporting.Period, bool) {
	prev := p
	switch p.Type {
	case "all":
		return prev, false
	case "monthly":
		prev.Start = p.Start.AddDate(0, -1, 0)
		prev.End = p.Start.AddDate(0, 0, -1)
	case "yearly":
		prev.Start = p.Start.AddDate(-1, 0, 0)
		prev.End = p.Start.AddDate(0, 0, -1)
	default:
		days := int((p.End.Unix()-p.Start.Unix())/86400) + 1
		prev.Start = p.Start.AddDate(0, 0, -days)
		prev.End = p.Start.AddDate(0, 0, -1)
	}
	return prev, prev.Start.Year() >= 1
}

// Get supplies opening/ending balances, equal calendar comparisons, category
// drilldown and six monthly trend points. Plans remain outside all these values.
func (s *FinancialReportService) Get(ctx context.Context, uid int64, code string, p reporting.Period) (FinancialReport, error) {
	out := FinancialReport{Currency: code, Categories: []ReportCategory{}, Trends: []ReportTrend{}}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	out.Categories, out.ReportTotals, err = reportCategories(ctx, q, uid, code, p)
	if err != nil {
		return out, err
	}
	var opening, ending string
	err = tx.QueryRow(ctx, `SELECT COALESCE(sum(p.amount_minor) FILTER(WHERE $3::timestamptz IS NOT NULL AND t.occurred_at<$3),0)::text,
 COALESCE(sum(p.amount_minor) FILTER(WHERE $4::timestamptz IS NULL OR t.occurred_at<$4),0)::text
 FROM transactions t JOIN postings p ON p.transaction_id=t.id JOIN accounts a ON a.id=p.account_id AND a.kind='normal'
 WHERE t.user_id=$1 AND t.currency=$2`, uid, code, pgtype.Timestamptz{Time: p.Start, Valid: p.Type != "all"}, pgtype.Timestamptz{Time: p.End.AddDate(0, 0, 1), Valid: p.Type != "all"}).Scan(&opening, &ending)
	if err != nil {
		return out, err
	}
	out.Opening, err = parseExactAggregate(opening)
	if err != nil {
		return out, err
	}
	out.Ending, err = parseExactAggregate(ending)
	if err != nil {
		return out, err
	}
	movement, err := addMinor(out.Ending, -out.Opening)
	if err != nil {
		return out, err
	}
	out.Other, err = addMinor(movement, -out.Net)
	if err != nil {
		return out, err
	}
	if p.Type != "all" {
		out.Start = p.Start.Format("2006-01-02")
		out.End = p.End.Format("2006-01-02")
	}
	if prev, ok := priorPeriod(p); ok {
		_, total, err := reportCategories(ctx, q, uid, code, prev)
		if err != nil {
			return out, err
		}
		out.Previous = &total
		change := ReportTotals{}
		change.Income, err = addMinor(out.Income, -total.Income)
		if err != nil {
			return out, err
		}
		change.Spending, err = addMinor(out.Spending, -total.Spending)
		if err != nil {
			return out, err
		}
		change.Net, err = addMinor(out.Net, -total.Net)
		if err != nil {
			return out, err
		}
		out.Change = &change
		out.PreviousStart = prev.Start.Format("2006-01-02")
		out.PreviousEnd = prev.End.Format("2006-01-02")
	}
	anchor := p.End
	if p.Type == "all" {
		anchor = time.Now().UTC()
	}
	anchor = time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.UTC)
	// One monthly aggregation keeps large histories from being scanned six times.
	trendRows, err := tx.Query(ctx, `SELECT to_char(t.occurred_at AT TIME ZONE 'UTC','YYYY-MM'),
 COALESCE(sum(CASE WHEN p.amount_minor>0 AND t.entry_type<>'refund' THEN p.amount_minor ELSE 0 END),0)::text,
 COALESCE(sum(CASE WHEN t.entry_type='refund' THEN -p.amount_minor WHEN p.amount_minor<0 THEN -p.amount_minor ELSE 0 END),0)::text
 FROM transactions t JOIN postings p ON p.transaction_id=t.id JOIN accounts a ON a.id=p.account_id AND a.kind='normal'
 WHERE t.user_id=$1 AND t.currency=$2 AND t.occurred_at>=$3 AND t.occurred_at<$4
 AND t.entry_type<>'opening_balance' AND NOT t.exclude_from_reports AND t.reversal_of IS NULL
 AND NOT EXISTS(SELECT 1 FROM transactions r WHERE r.reversal_of=t.id)
 AND EXISTS(SELECT 1 FROM postings ep JOIN accounts ea ON ea.id=ep.account_id WHERE ep.transaction_id=t.id AND ea.kind='external')
 GROUP BY 1 ORDER BY 1`, uid, code, anchor.AddDate(0, -5, 0), anchor.AddDate(0, 1, 0))
	if err != nil {
		return out, err
	}
	byMonth := map[string]ReportTotals{}
	for trendRows.Next() {
		var month, income, spent string
		if err := trendRows.Scan(&month, &income, &spent); err != nil {
			trendRows.Close()
			return out, err
		}
		total := ReportTotals{}
		total.Income, err = parseExactAggregate(income)
		if err != nil {
			trendRows.Close()
			return out, err
		}
		total.Spending, err = parseExactAggregate(spent)
		if err != nil {
			trendRows.Close()
			return out, err
		}
		total.Net, err = addMinor(total.Income, -total.Spending)
		if err != nil {
			trendRows.Close()
			return out, err
		}
		byMonth[month] = total
	}
	err = trendRows.Err()
	trendRows.Close()
	if err != nil {
		return out, err
	}
	for i := 5; i >= 0; i-- {
		at := anchor.AddDate(0, -i, 0)
		if at.Year() >= 1 {
			month := at.Format("2006-01")
			out.Trends = append(out.Trends, ReportTrend{Month: month, ReportTotals: byMonth[month]})
		}
	}
	return out, tx.Commit(ctx)
}
