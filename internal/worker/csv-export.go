package worker

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) handleExportCSV(ctx context.Context, t *asynq.Task) error {
	var p ExportCSVPayload
	if err := jsonUnmarshal(t.Payload(), &p); err != nil {
		return err
	}
	month, _ := time.Parse("2006-01-02", p.Month)

	// Query transactions
	rows, err := s.q.ListTransactionsForMonth(ctx, sqlc.ListTransactionsForMonthParams{
		UserID:  p.UserID,
		Column2: month,
	})
	if err != nil {
		// mark as error
		_ = s.q.UpdateExportStatus(ctx, sqlc.UpdateExportStatusParams{
			ID:       p.ExportID,
			Status:   "error",
			FilePath: pgtype.Text{Valid: false}, // NULL
		})
		return err
	}

	// Write CSV
	dir := "./tmp/exports"
	_ = os.MkdirAll(dir, 0755)

	filePath := filepath.Join(dir, fmt.Sprintf("export_%d.csv", p.ExportID))
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	_ = w.Write([]string{"id", "account_id", "category_id", "amount_minor", "currency", "occurred_at", "note"})
	for _, tr := range rows {
		cat := ""
		if tr.CategoryID.Valid { // CategoryID is pgtype.Int8
			cat = fmt.Sprint(tr.CategoryID.Int64)
		}
		note := ""
		if tr.Note.Valid { // Note is pgtype.Text
			note = tr.Note.String
		}

		_ = w.Write([]string{
			fmt.Sprint(tr.ID),
			fmt.Sprint(tr.AccountID), // plain int64
			cat,
			fmt.Sprint(tr.AmountMinor),
			tr.Currency,
			tr.OccurredAt.Format(time.RFC3339),
			note,
		})
	}
	w.Flush()

	// Update status → set file_path
	return s.q.UpdateExportStatus(ctx, sqlc.UpdateExportStatusParams{
		ID:       p.ExportID,
		Status:   "done",
		FilePath: pgtype.Text{String: filePath, Valid: true},
	})
}

func jsonUnmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}
