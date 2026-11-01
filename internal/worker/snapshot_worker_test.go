package worker

import (
	"context"
	"errors"
	"testing"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
)

type snapshotRaceQueries struct {
	sqlc.Querier
	missing bool
	failure error
}

func (q *snapshotRaceQueries) SumPostingsUpTo(context.Context, sqlc.SumPostingsUpToParams) (int64, error) {
	return 0, nil
}
func (q *snapshotRaceQueries) UpsertBalanceSnapshot(context.Context, sqlc.UpsertBalanceSnapshotParams) error {
	return q.failure
}
func (q *snapshotRaceQueries) GetAccount(context.Context, int64) (sqlc.GetAccountRow, error) {
	if q.missing {
		return sqlc.GetAccountRow{}, pgx.ErrNoRows
	}
	return sqlc.GetAccountRow{ID: 1}, nil
}

func TestSnapshotJobSkipsAccountReplacedByRestoreButPreservesOtherFailures(t *testing.T) {
	failure := errors.New("snapshot write failed")
	for _, missing := range []bool{true, false} {
		queries := &snapshotRaceQueries{missing: missing, failure: failure}
		s := Server{q: queries}
		err := s.handleSnapshotAccount(context.Background(), NewTaskSnapshotAccount(SnapshotAccountPayload{AccountID: 1, Date: "2024-01-01"}))
		if missing && err != nil {
			t.Fatal("deleted system account job kept retrying", err)
		}
		if !missing && !errors.Is(err, failure) {
			t.Fatal("real snapshot error was swallowed", err)
		}
	}
}
