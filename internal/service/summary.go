package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/redis/go-redis/v9"
)

// SummaryService caches account summaries by UTC month.
type SummaryService struct {
	q   *sqlc.Queries
	rdb *redis.Client
	ttl time.Duration
}

// NewSummaryService creates a summary reader with a five-minute Redis cache.
func NewSummaryService(q *sqlc.Queries, rdb *redis.Client) *SummaryService {
	return &SummaryService{q: q, rdb: rdb, ttl: 5 * time.Minute}
}

// MonthlySummary contains inflow, outflow, and net amounts in minor units.
type MonthlySummary struct {
	Inflow  int64 `json:"inflow"`
	Outflow int64 `json:"outflow"`
	Net     int64 `json:"net"`
}

// GetMonthlySummary returns account totals for the UTC month containing month.
func (s *SummaryService) GetMonthlySummary(ctx context.Context, accountID int64, month time.Time) (MonthlySummary, error) {
	month = utcMonthStart(month)
	key := fmt.Sprintf("summary:%d:%s", accountID, month.Format("2006-01"))

	// 1) Try Redis
	val, err := s.rdb.Get(ctx, key).Result()
	if err == nil {
		var ms MonthlySummary
		_ = json.Unmarshal([]byte(val), &ms)
		return ms, nil
	}

	// 2) Query DB
	row, err := s.q.GetMonthlySummary(ctx, sqlc.GetMonthlySummaryParams{
		AccountID:  accountID,
		MonthStart: month,
		MonthEnd:   month.AddDate(0, 1, 0),
	})
	if err != nil {
		return MonthlySummary{}, err
	}
	ms := MonthlySummary{
		Inflow:  row.Inflow,
		Outflow: row.Outflow,
		Net:     row.Net,
	}

	// 3) Store in Redis
	b, _ := json.Marshal(ms)
	_ = s.rdb.Set(ctx, key, b, s.ttl).Err()

	return ms, nil
}

// Invalidate removes the cached account summary for the UTC month containing month.
func (s *SummaryService) Invalidate(ctx context.Context, accountID int64, month time.Time) {
	month = utcMonthStart(month)
	key := fmt.Sprintf("summary:%d:%s", accountID, month.Format("2006-01"))
	_ = s.rdb.Del(ctx, key).Err()
}

func utcMonthStart(at time.Time) time.Time {
	at = at.UTC()
	return time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
}
