package worker

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) handleExportCSV(ctx context.Context, t *asynq.Task) error {
	var p ExportCSVPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	month, err := time.Parse("2006-01-02", p.Month)
	if err != nil {
		s.failExport(ctx, p.ExportID)
		return err
	}

	rows, err := s.q.ListPostingsForMonth(ctx, sqlc.ListPostingsForMonthParams{
		UserID:  p.UserID,
		Column2: month,
	})
	if err != nil {
		s.failExport(ctx, p.ExportID)
		return err
	}

	// Exports are one user-month, small enough to build in memory; a
	// seekable buffer is also what the S3 client needs for signing.
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"transaction_id", "account_id", "account_name", "category_id", "amount_minor", "currency", "occurred_at", "note"})
	for _, tr := range rows {
		cat := ""
		if tr.CategoryID.Valid {
			cat = fmt.Sprint(tr.CategoryID.Int64)
		}
		note := ""
		if tr.Note.Valid {
			note = tr.Note.String
		}
		_ = w.Write([]string{
			fmt.Sprint(tr.TransactionID),
			fmt.Sprint(tr.AccountID),
			tr.AccountName,
			cat,
			fmt.Sprint(tr.AmountMinor),
			tr.Currency,
			tr.OccurredAt.Format(time.RFC3339),
			note,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		s.failExport(ctx, p.ExportID)
		return err
	}

	key := fmt.Sprintf("export_%d.csv", p.ExportID)
	if err := s.store.Save(ctx, key, bytes.NewReader(buf.Bytes())); err != nil {
		s.failExport(ctx, p.ExportID)
		return err
	}

	return s.q.UpdateExportStatus(ctx, sqlc.UpdateExportStatusParams{
		ID:       p.ExportID,
		Status:   "done",
		FilePath: pgtype.Text{String: key, Valid: true},
	})
}

func (s *Server) failExport(ctx context.Context, exportID int64) {
	_ = s.q.UpdateExportStatus(ctx, sqlc.UpdateExportStatusParams{
		ID:       exportID,
		Status:   "error",
		FilePath: pgtype.Text{Valid: false},
	})
}
