package integration

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/reporting"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestReportStreamingPeriodsAndOwnership(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	otherUID, otherAccount := newUserWithAccount(t, "USD")
	dates := []string{"2023-12-31T23:59:59Z", "2024-01-01T00:00:00Z", "2024-02-29T23:59:59.999999Z", "2024-03-01T00:00:00Z", "2024-12-31T23:59:59Z", "2025-01-01T00:00:00Z"}
	ids := make([]int64, 0, len(dates))
	for _, value := range dates {
		at, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: 100, Currency: "USD", OccurredAt: at})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tx.ID)
	}
	if _, err := txSvc.Create(ctx, service.CreateInput{UserID: otherUID, AccountID: otherAccount, AmountMinor: 999, Currency: "USD", OccurredAt: time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
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
	queries := q.WithTx(tx)
	cases := []struct {
		input reporting.Selection
		want  []int64
	}{
		{reporting.Selection{Month: "2024-02"}, ids[2:3]},
		{reporting.Selection{Type: "weekly", WeekOf: "2024-01-02"}, ids[1:2]},
		{reporting.Selection{Type: "yearly", Year: "2024"}, ids[1:5]},
		{reporting.Selection{Type: "custom", StartDate: "2024-02-29", EndDate: "2024-02-29"}, ids[2:3]},
		{reporting.Selection{Type: "all"}, ids},
	}
	for _, tc := range cases {
		period, err := reporting.Parse(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		params := sqlc.ListPostingsForExportParams{UserID: uid, StartDate: pgtype.Date{Time: period.Start, Valid: period.Type != "all"}, EndDate: pgtype.Date{Time: period.End, Valid: period.Type != "all"}}
		var got []int64
		if err := queries.StreamExportPostings(ctx, params, func(row sqlc.ListPostingsForExportRow) error { got = append(got, row.TransactionID); return nil }); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s report got %v, want %v", period.Type, got, tc.want)
		}
		report, err := queries.CreateExport(ctx, sqlc.CreateExportParams{UserID: uid, ReportType: period.Type, Month: pgtype.Date{Time: period.Start, Valid: period.Month != ""}, StartDate: params.StartDate, EndDate: params.EndDate})
		if err != nil {
			t.Fatal(err)
		}
		if report.ReportType != period.Type || report.StartDate.Valid != params.StartDate.Valid || report.EndDate.Valid != params.EndDate.Valid {
			t.Fatalf("range metadata lost: %+v", report)
		}
	}
	wantErr := errors.New("output file full")
	if err := queries.StreamExportPostings(ctx, sqlc.ListPostingsForExportParams{UserID: uid}, func(sqlc.ListPostingsForExportRow) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("stream swallowed consumer error: %v", err)
	}
}
