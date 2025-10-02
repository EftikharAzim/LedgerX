package service

import (
	"context"
	"strconv"
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

func (s *BalanceService) CurrentBalance(ctx context.Context, accountID int64, now time.Time) (BalanceResult, error) {
	var snapCutoff time.Time
	var snapBal int64

	// Try to fetch the latest snapshot for this account.
	snap, err := s.q.GetLatestSnapshot(ctx, accountID)
	if err == nil {
		// as_of_date is now time.Time thanks to sqlc override
		d := snap.AsOfDate.UTC()
		// end-of-day cutoff (23:59:59.999... UTC)
		snapCutoff = time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
		snapBal = snap.BalanceMinor
	} else {
		// No snapshot yet: consider cutoff = zero time, base balance = 0
		snapCutoff = time.Time{}
		snapBal = 0
	}

	deltaRaw, err := s.q.SumTransactionsSince(ctx, sqlc.SumTransactionsSinceParams{
		AccountID: accountID,
		Since:     snapCutoff, // time.Time
	})
	if err != nil {
		return BalanceResult{}, err
	}
	delta := toInt64(deltaRaw)

	return BalanceResult{
		AccountID: accountID,
		AsOfISO:   now.UTC().Format(time.RFC3339),
		Balance:   snapBal + delta,
	}, nil
}

// toInt64 converts sqlc's interface{} sum result into an int64.
// sqlc may return int64, int32, string, or []byte depending on driver/value.
func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int32:
		return int64(t)
	case int:
		return int64(t)
	case []byte:
		if n, err := strconv.ParseInt(string(t), 10, 64); err == nil {
			return n
		}
	case string:
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			return n
		}
	default:
		return 0
	}
	return 0
}
