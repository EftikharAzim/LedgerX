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
	q   summaryQueries
	rdb *redis.Client
	ttl time.Duration
}

type summaryQueries interface {
	GetMonthlySummary(context.Context, sqlc.GetMonthlySummaryParams) (sqlc.GetMonthlySummaryRow, error)
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

	if val, err := s.rdb.Get(ctx, key).Result(); err == nil {
		if ms, ok := cachedMonthlySummary(val); ok {
			return ms, nil
		}
	}

	var ms MonthlySummary
	var loadErr error
	obtained := false
	load := func() {
		row, err := s.q.GetMonthlySummary(ctx, sqlc.GetMonthlySummaryParams{
			AccountID: accountID, MonthStart: month, MonthEnd: month.AddDate(0, 1, 0),
		})
		ms = MonthlySummary{Inflow: row.Inflow, Outflow: row.Outflow, Net: row.Net}
		loadErr = err
		if err == nil && !exactMonthlySummary(ms) {
			loadErr = ErrMoneyRange
		}
	}
	// Watch before reading Postgres. A concurrent invalidation changes the
	// watched key, so an older read cannot repopulate a newly invalidated cache.
	// Cache outages and WATCH conflicts never invalidate a successful DB read.
	_ = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		obtained = true
		if val, err := tx.Get(ctx, key).Result(); err == nil {
			if cached, ok := cachedMonthlySummary(val); ok {
				ms = cached
				return nil
			}
		}
		load()
		if loadErr != nil {
			return loadErr
		}
		b, err := json.Marshal(ms)
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, b, s.ttl)
			return nil
		})
		return err
	}, key)
	if !obtained {
		load()
	}
	return ms, loadErr
}

func exactMonthlySummary(ms MonthlySummary) bool {
	return ms.Inflow >= 0 && ms.Outflow >= 0 && safeMinor(ms.Inflow) && safeMinor(ms.Outflow) && safeMinor(ms.Net) && ms.Inflow-ms.Outflow == ms.Net
}

func cachedMonthlySummary(value string) (MonthlySummary, bool) {
	// Pointer fields distinguish a real zero summary from null or a partial,
	// malformed cache value. Invalid caches fall through to authoritative SQL.
	var cached struct {
		Inflow  *int64 `json:"inflow"`
		Outflow *int64 `json:"outflow"`
		Net     *int64 `json:"net"`
	}
	if err := json.Unmarshal([]byte(value), &cached); err != nil || cached.Inflow == nil || cached.Outflow == nil || cached.Net == nil {
		return MonthlySummary{}, false
	}
	ms := MonthlySummary{Inflow: *cached.Inflow, Outflow: *cached.Outflow, Net: *cached.Net}
	if exactMonthlySummary(ms) {
		return ms, true
	}
	return MonthlySummary{}, false
}

// Invalidate expires a summary and changes even a previously absent cache key.
// DEL on a missing key cannot invalidate an in-flight WATCH-based cache fill.
func (s *SummaryService) Invalidate(ctx context.Context, accountID int64, month time.Time) {
	month = utcMonthStart(month)
	key := fmt.Sprintf("summary:%d:%s", accountID, month.Format("2006-01"))
	_ = s.rdb.Set(ctx, key, "", s.ttl).Err()
}

func utcMonthStart(at time.Time) time.Time {
	at = at.UTC()
	return time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
}
