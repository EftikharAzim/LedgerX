package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/money"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
)

type balanceStub struct {
	snapshot  sqlc.BalanceSnapshot
	err       error
	delta     int64
	deltaRead bool
}

func (s *balanceStub) GetLatestSnapshot(context.Context, int64) (sqlc.BalanceSnapshot, error) {
	return s.snapshot, s.err
}
func (s *balanceStub) SumPostingsSince(context.Context, sqlc.SumPostingsSinceParams) (int64, error) {
	s.deltaRead = true
	return s.delta, nil
}

func TestBalanceSnapshotErrorsAreNotTreatedAsMissing(t *testing.T) {
	want := errors.New("snapshot read unavailable")
	q := &balanceStub{err: want, delta: 100}
	_, err := (&BalanceService{q: q}).CurrentBalance(context.Background(), 1, time.Now())
	if !errors.Is(err, want) || q.deltaRead {
		t.Fatalf("snapshot failure was hidden: err=%v delta read=%v", err, q.deltaRead)
	}
}

func TestBalanceExactRangeWithAndWithoutSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name            string
		snapshot, delta int64
		missing         bool
		wantErr         bool
	}{
		{"no snapshot", 0, 125, true, false},
		{"snapshot plus delta", 100, -25, false, false},
		{"positive overflow", money.MaxSafeMinor, 1, false, true},
		{"negative overflow", -money.MaxSafeMinor, -1, false, true},
		{"unsafe legacy total", 0, money.MaxSafeMinor + 1, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &balanceStub{snapshot: sqlc.BalanceSnapshot{AsOfDate: time.Now().UTC().AddDate(0, 0, -1), BalanceMinor: tc.snapshot}, delta: tc.delta}
			if tc.missing {
				q.err = pgx.ErrNoRows
			}
			result, err := (&BalanceService{q: q}).CurrentBalance(context.Background(), 1, time.Now())
			if tc.wantErr {
				if !errors.Is(err, ErrMoneyRange) {
					t.Fatalf("unsupported balance: %+v %v", result, err)
				}
			} else if err != nil || result.Balance != tc.snapshot+tc.delta {
				t.Fatalf("balance: %+v %v", result, err)
			}
		})
	}
}
