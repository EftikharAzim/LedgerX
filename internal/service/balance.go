package service

import (
	"context"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
)

type BalanceService struct{ q *sqlc.Queries }

func NewBalanceService(q *sqlc.Queries) *BalanceService { return &BalanceService{q: q} }

type BalanceResult struct {
	AccountID int64  `json:"account_id"`
	AsOfISO   string `json:"as_of"`
	Balance   int64  `json:"balance_minor"`
}

// CurrentBalance = latest snapshot + postings created after the snapshot
// cutoff. Both sides cut on postings.created_at, so a backdated entry
// (occurred_at in the past, created today) still lands in the delta.
func (s *BalanceService) CurrentBalance(ctx context.Context, accountID int64, now time.Time) (BalanceResult, error) {
	var snapCutoff time.Time
	var snapBal int64

	snap, err := s.q.GetLatestSnapshot(ctx, accountID)
	if err == nil {
		d := snap.AsOfDate.UTC()
		// end-of-day cutoff matching the snapshot worker's SumPostingsUpTo
		snapCutoff = time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
		snapBal = snap.BalanceMinor
	}

	delta, err := s.q.SumPostingsSince(ctx, sqlc.SumPostingsSinceParams{
		AccountID: accountID,
		Since:     snapCutoff,
	})
	if err != nil {
		return BalanceResult{}, err
	}

	return BalanceResult{
		AccountID: accountID,
		AsOfISO:   now.UTC().Format(time.RFC3339),
		Balance:   snapBal + delta,
	}, nil
}
