package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/money"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/redis/go-redis/v9"
)

type summaryQueryFunc func(context.Context, sqlc.GetMonthlySummaryParams) (sqlc.GetMonthlySummaryRow, error)

func (f summaryQueryFunc) GetMonthlySummary(ctx context.Context, p sqlc.GetMonthlySummaryParams) (sqlc.GetMonthlySummaryRow, error) {
	return f(ctx, p)
}

func summaryTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set")
	}
	// Separate from integration-test data and rate-limit test buckets.
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestSummaryCorruptCacheFallsBackToAuthoritativeTotals(t *testing.T) {
	rdb := summaryTestRedis(t)
	ctx := context.Background()
	month := utcMonthStart(time.Now())
	key := fmt.Sprintf("summary:101:%s", month.Format("2006-01"))
	t.Cleanup(func() { _ = rdb.Del(ctx, key).Err() })
	svc := &SummaryService{rdb: rdb, ttl: time.Minute, q: summaryQueryFunc(func(context.Context, sqlc.GetMonthlySummaryParams) (sqlc.GetMonthlySummaryRow, error) {
		return sqlc.GetMonthlySummaryRow{Inflow: 100, Outflow: 25, Net: 75}, nil
	})}
	for _, value := range []string{"broken JSON", "null", "{}", `{"inflow":100,"outflow":25}`, `{"inflow":100,"outflow":25,"net":0}`, `{"inflow":9007199254740992,"outflow":0,"net":9007199254740992}`} {
		if err := rdb.Set(ctx, key, value, time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
		got, err := svc.GetMonthlySummary(ctx, 101, month)
		if err != nil || got != (MonthlySummary{Inflow: 100, Outflow: 25, Net: 75}) {
			t.Fatalf("corrupt cache %q masked SQL totals: %+v %v", value, got, err)
		}
	}
}

func TestSummaryInvalidationWinsAnInFlightCacheMiss(t *testing.T) {
	rdb := summaryTestRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	month := utcMonthStart(time.Now())
	key := fmt.Sprintf("summary:102:%s", month.Format("2006-01"))
	if err := rdb.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Del(context.Background(), key).Err() })
	started, release := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	svc := &SummaryService{rdb: rdb, ttl: time.Minute, q: summaryQueryFunc(func(ctx context.Context, _ sqlc.GetMonthlySummaryParams) (sqlc.GetMonthlySummaryRow, error) {
		if reads.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return sqlc.GetMonthlySummaryRow{}, ctx.Err()
			}
			return sqlc.GetMonthlySummaryRow{Inflow: 100, Net: 100}, nil
		}
		return sqlc.GetMonthlySummaryRow{Inflow: 200, Net: 200}, nil
	})}
	finished := make(chan error, 1)
	go func() { _, err := svc.GetMonthlySummary(ctx, 102, month); finished <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// The key was absent when the read started. A plain DEL would not change
	// Redis's WATCH state and would let the older read resurrect stale totals.
	svc.Invalidate(ctx, 102, month)
	close(release)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	got, err := svc.GetMonthlySummary(ctx, 102, month)
	if err != nil || got.Net != 200 || reads.Load() != 2 {
		t.Fatalf("old in-flight read repopulated cache: %+v reads=%d err=%v", got, reads.Load(), err)
	}
}

func TestSummaryCacheOutageAndExactRange(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "unused", MaxRetries: -1, Dialer: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("cache offline")
	}})
	defer func() { _ = rdb.Close() }()
	for _, unsafe := range []bool{false, true} {
		svc := &SummaryService{rdb: rdb, q: summaryQueryFunc(func(context.Context, sqlc.GetMonthlySummaryParams) (sqlc.GetMonthlySummaryRow, error) {
			value := int64(100)
			if unsafe {
				value = money.MaxSafeMinor + 1
			}
			return sqlc.GetMonthlySummaryRow{Inflow: value, Net: value}, nil
		})}
		got, err := svc.GetMonthlySummary(context.Background(), 1, time.Now())
		if unsafe {
			if !errors.Is(err, ErrMoneyRange) {
				t.Fatalf("unsafe JSON total returned: %+v %v", got, err)
			}
		} else if err != nil || got.Net != 100 {
			t.Fatalf("cache outage masked successful SQL read: %+v %v", got, err)
		}
	}
}
