package integration

import (
	"context"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
)

func TestExportMonthUsesUTCWithNonUTCSession(t *testing.T) {
	ctx := context.Background()
	uid, accID := newUserWithAccount(t, "USD")
	for _, occurred := range []time.Time{
		time.Date(2026, time.September, 30, 23, 59, 0, 0, time.UTC),
		time.Date(2026, time.October, 31, 23, 59, 0, 0, time.UTC),
	} {
		if _, err := txSvc.Create(ctx, service.CreateInput{
			UserID: uid, AccountID: accID, AmountMinor: 100, Currency: "USD", OccurredAt: occurred,
		}); err != nil {
			t.Fatal(err)
		}
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL TIME ZONE 'Asia/Dhaka'"); err != nil {
		t.Fatal(err)
	}
	rows, err := q.WithTx(tx).ListPostingsForMonth(ctx, sqlc.ListPostingsForMonthParams{
		UserID: uid, Column2: time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].OccurredAt.Equal(time.Date(2026, time.October, 31, 23, 59, 0, 0, time.UTC)) {
		t.Fatalf("UTC October export must include October boundary and exclude September: %+v", rows)
	}
}
