package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestLedgerRejectsPostingMutations(t *testing.T) {
	ctx := context.Background()
	uid, accountID := newUserWithAccount(t, "USD")
	original, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accountID, AmountMinor: 100,
		Currency: "USD", OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accountID, AmountMinor: 200,
		Currency: "USD", OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		sql  string
		args []any
	}{
		{"move posting with balanced destination", `
			WITH moved AS (
				UPDATE postings SET transaction_id=$1 WHERE id=$2
				RETURNING transaction_id, account_id, amount_minor
			)
			INSERT INTO postings (transaction_id, account_id, amount_minor)
			SELECT transaction_id, account_id, -amount_minor FROM moved`, []any{other.ID, original.Postings[0].ID}},
		{"edit balanced amounts", `UPDATE postings SET amount_minor=amount_minor*2 WHERE transaction_id=$1`, []any{original.ID}},
		{"delete all legs", `DELETE FROM postings WHERE transaction_id=$1`, []any{original.ID}},
		{"delete header", `DELETE FROM transactions WHERE id=$1`, []any{original.ID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, tc.sql, tc.args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "55000" {
				t.Fatalf("want append-only rejection (55000), got %v", err)
			}
		})
	}

	postings, err := q.ListPostingsByTransaction(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(postings) != 2 || postings[0].AmountMinor != 100 || postings[1].AmountMinor != -100 {
		t.Fatalf("rejected mutations changed original postings: %+v", postings)
	}
	if _, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: original.ID}); err != nil {
		t.Fatalf("append-only protection must allow linked reversal: %v", err)
	}
}

func TestLedgerAllowsUserRetentionCascade(t *testing.T) {
	ctx := context.Background()
	uid, accountID := newUserWithAccount(t, "USD")
	original, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accountID, AmountMinor: 100,
		Currency: "USD", OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: original.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid); err != nil {
		t.Fatalf("existing user-retention cascade must remain available: %v", err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM postings WHERE account_id=$1`, accountID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("retention deletion left %d postings", remaining)
	}
}

func TestSummaryInvalidatesUTCMonthForOffsetTimestamps(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		occurredAt string
		month      string
	}{
		{"east of UTC", "2026-06-01T00:30:00+06:00", "2026-05"},
		{"west of UTC", "2026-05-31T23:30:00-05:00", "2026-06"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uid, accountID := newUserWithAccount(t, "USD")
			month, err := time.Parse("2006-01", tc.month)
			if err != nil {
				t.Fatal(err)
			}
			// Schema resets reuse account IDs while Redis may outlive a test
			// run. Remove this fixture's key before warming the initial cache.
			key := fmt.Sprintf("summary:%d:%s", accountID, tc.month)
			if err := rdb.Del(ctx, key).Err(); err != nil {
				t.Fatal(err)
			}
			initial, err := sumSvc.GetMonthlySummary(ctx, accountID, month)
			if err != nil || initial.Net != 0 {
				t.Fatalf("initial summary: %+v, %v", initial, err)
			}
			if exists, err := rdb.Exists(ctx, key).Result(); err != nil || exists != 1 {
				t.Fatalf("test requires a cached summary: exists=%d, err=%v", exists, err)
			}
			at, err := time.Parse(time.RFC3339, tc.occurredAt)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := txSvc.Create(ctx, service.CreateInput{
				UserID: uid, AccountID: accountID, AmountMinor: 150,
				Currency: "USD", OccurredAt: at,
			}); err != nil {
				t.Fatal(err)
			}
			updated, err := sumSvc.GetMonthlySummary(ctx, accountID, month)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Inflow != 150 || updated.Net != 150 {
				t.Fatalf("summary remained stale across UTC month boundary: %+v", updated)
			}
		})
	}
}

func TestSummaryUTCMonthIgnoresDatabaseTimezone(t *testing.T) {
	ctx := context.Background()
	uid, accountID := newUserWithAccount(t, "USD")
	for _, tc := range []struct {
		at     string
		amount int64
	}{
		{"2026-05-01T00:30:00Z", 100},
		{"2026-05-31T23:30:00Z", 50},
		{"2026-06-01T00:30:00Z", 200},
	} {
		at, err := time.Parse(time.RFC3339, tc.at)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := txSvc.Create(ctx, service.CreateInput{
			UserID: uid, AccountID: accountID, AmountMinor: tc.amount,
			Currency: "USD", OccurredAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL TIME ZONE 'Asia/Dhaka'`); err != nil {
		t.Fatal(err)
	}
	month := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	row, err := q.WithTx(tx).GetMonthlySummary(ctx, sqlc.GetMonthlySummaryParams{
		AccountID: accountID, MonthStart: month, MonthEnd: month.AddDate(0, 1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Inflow != 150 || row.Outflow != 0 || row.Net != 150 {
		t.Fatalf("database timezone changed UTC month membership: %+v", row)
	}
}
