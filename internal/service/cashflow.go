package service

import (
	"context"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/planning"
	"github.com/jackc/pgx/v5"
)

// CashflowDay is a day-level estimate from pending commitments.
type CashflowDay struct {
	Date           string `json:"date"`
	InMinor        int64  `json:"in_minor"`
	OutMinor       int64  `json:"out_minor"`
	ProjectedMinor int64  `json:"projected_minor"`
	Count          int64  `json:"count"`
}

// WalletCashflow keeps unlike currencies and separate wallet obligations apart.
type WalletCashflow struct {
	AccountID      int64         `json:"account_id"`
	Name           string        `json:"name"`
	Currency       string        `json:"currency"`
	Active         bool          `json:"active"`
	CurrentMinor   int64         `json:"current_minor"`
	InMinor        int64         `json:"in_minor"`
	OutMinor       int64         `json:"out_minor"`
	ProjectedMinor int64         `json:"projected_minor"`
	LowestMinor    int64         `json:"lowest_minor"`
	OverdueCount   int64         `json:"overdue_count"`
	Days           []CashflowDay `json:"days"`
}

// Cashflow is a 30-day planning view, independent of posted reports.
type Cashflow struct {
	AsOf    string           `json:"as_of"`
	Through string           `json:"through_date"`
	Wallets []WalletCashflow `json:"wallets"`
}

// Cashflow estimates day-end balances; overdue plans are assumed to happen today.
func (s *ScheduleService) Cashflow(ctx context.Context, uid int64, now time.Time) (Cashflow, error) {
	today := planning.Today(now)
	through := today.AddDate(0, 0, 29)
	out := Cashflow{AsOf: today.Format("2006-01-02"), Through: through.Format("2006-01-02"), Wallets: []WalletCashflow{}}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT a.id,a.name,a.currency,a.is_active,COALESCE(sum(p.amount_minor),0)::text FROM accounts a
 LEFT JOIN postings p ON p.account_id=a.id WHERE a.user_id=$1 AND a.kind='normal' GROUP BY a.id ORDER BY a.id`, uid)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var w WalletCashflow
		var amount string
		if err := rows.Scan(&w.AccountID, &w.Name, &w.Currency, &w.Active, &amount); err != nil {
			rows.Close()
			return out, err
		}
		w.CurrentMinor, err = parseExactAggregate(amount)
		if err != nil {
			rows.Close()
			return out, err
		}
		w.ProjectedMinor = w.CurrentMinor
		w.LowestMinor = w.CurrentMinor
		w.Days = []CashflowDay{}
		out.Wallets = append(out.Wallets, w)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, `SELECT account_id,GREATEST((occurred_at AT TIME ZONE 'UTC')::date,$2::date)::text,
 COALESCE(sum(CASE WHEN amount_minor>0 THEN amount_minor ELSE 0 END),0)::text,
 COALESCE(sum(CASE WHEN amount_minor<0 THEN -amount_minor ELSE 0 END),0)::text,count(*),count(*) FILTER(WHERE occurred_at<$2::date::timestamp AT TIME ZONE 'UTC')
 FROM planned_transactions WHERE user_id=$1 AND status='pending' AND occurred_at<($3::date+1)::timestamp AT TIME ZONE 'UTC'
 GROUP BY account_id,2 ORDER BY 2,account_id`, uid, today, through)
	if err != nil {
		return out, err
	}
	index := map[int64]int{}
	for i, w := range out.Wallets {
		index[w.AccountID] = i
	}
	for rows.Next() {
		var aid, overdue int64
		var d CashflowDay
		var incoming, outgoing string
		if err := rows.Scan(&aid, &d.Date, &incoming, &outgoing, &d.Count, &overdue); err != nil {
			rows.Close()
			return out, err
		}
		i, ok := index[aid]
		if !ok {
			continue
		}
		w := &out.Wallets[i]
		d.InMinor, err = parseExactAggregate(incoming)
		if err != nil {
			rows.Close()
			return out, err
		}
		d.OutMinor, err = parseExactAggregate(outgoing)
		if err != nil {
			rows.Close()
			return out, err
		}
		w.InMinor, err = addMinor(w.InMinor, d.InMinor)
		if err != nil {
			rows.Close()
			return out, err
		}
		w.OutMinor, err = addMinor(w.OutMinor, d.OutMinor)
		if err != nil {
			rows.Close()
			return out, err
		}
		delta, err := addMinor(d.InMinor, -d.OutMinor)
		if err != nil {
			rows.Close()
			return out, err
		}
		w.ProjectedMinor, err = addMinor(w.ProjectedMinor, delta)
		if err != nil {
			rows.Close()
			return out, err
		}
		d.ProjectedMinor = w.ProjectedMinor
		if w.ProjectedMinor < w.LowestMinor {
			w.LowestMinor = w.ProjectedMinor
		}
		w.OverdueCount += overdue
		w.Days = append(w.Days, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
