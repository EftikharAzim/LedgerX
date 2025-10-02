package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/redis/go-redis/v9"
)

type SummaryService struct {
	q   *sqlc.Queries
	rdb *redis.Client
	ttl time.Duration
}

func NewSummaryService(q *sqlc.Queries, rdb *redis.Client) *SummaryService {
	return &SummaryService{q: q, rdb: rdb, ttl: 5 * time.Minute}
}

type MonthlySummary struct {
	Inflow  int64 `json:"inflow"`
	Outflow int64 `json:"outflow"`
	Net     int64 `json:"net"`
}

func (s *SummaryService) GetMonthlySummary(ctx context.Context, accountID int64, month time.Time) (MonthlySummary, error) {
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
		AccountID: accountID,
		Column2:   month,
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

// Bust cache when new tx happens
func (s *SummaryService) Invalidate(ctx context.Context, accountID int64, month time.Time) {
	key := fmt.Sprintf("summary:%d:%s", accountID, month.Format("2006-01"))
	_ = s.rdb.Del(ctx, key).Err()
}
