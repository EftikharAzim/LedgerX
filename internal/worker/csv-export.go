package worker

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) handleExportCSV(ctx context.Context, t *asynq.Task) error {
	var p ExportCSVPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("invalid export payload: %v: %w", err, asynq.SkipRetry)
	}
	if p.ExportID <= 0 || p.UserID <= 0 {
		return fmt.Errorf("invalid export identity: %w", asynq.SkipRetry)
	}
	if p.Month != "" {
		if _, err := time.Parse("2006-01-02", p.Month); err != nil {
			return fmt.Errorf("invalid legacy export month: %v: %w", err, asynq.SkipRetry)
		}
	}
	report, err := s.q.GetExportByID(ctx, p.ExportID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("export not found: %w", asynq.SkipRetry)
	}
	if err != nil {
		return err
	}
	if report.UserID != p.UserID {
		return fmt.Errorf("export owner mismatch: %w", asynq.SkipRetry)
	}
	// A seekable temporary file keeps both rows and CSV bytes out of RAM,
	// including all-time reports. CreateTemp restricts the file to mode 0600.
	file, err := os.CreateTemp("", "ledgerx-export-*.csv")
	if err != nil {
		return err
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()) }()
	w := csv.NewWriter(file)
	if err := w.Write([]string{"transaction_id", "account_id", "account_name", "category_id", "amount_minor", "currency", "occurred_at", "note"}); err != nil {
		return err
	}
	err = s.streamExport(ctx, sqlc.ListPostingsForExportParams{UserID: report.UserID, StartDate: report.StartDate, EndDate: report.EndDate}, func(row sqlc.ListPostingsForExportRow) error {
		category, note := "", ""
		if row.CategoryID.Valid {
			category = fmt.Sprint(row.CategoryID.Int64)
		}
		if row.Note.Valid {
			note = row.Note.String
		}
		return w.Write([]string{fmt.Sprint(row.TransactionID), fmt.Sprint(row.AccountID), row.AccountName, category, fmt.Sprint(row.AmountMinor), row.Currency, row.OccurredAt.Format(time.RFC3339), note})
	})
	if err != nil {
		return err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	key := fmt.Sprintf("export_%d.csv", p.ExportID)
	if err := s.store.Save(ctx, key, file); err != nil {
		return err
	}
	return s.q.UpdateExportStatus(ctx, sqlc.UpdateExportStatusParams{ID: p.ExportID, Status: "done", FilePath: pgtype.Text{String: key, Valid: true}})
}

// handleTaskError leaves retryable exports pending so clients keep polling.
// Asynq calls it with the attempt metadata before retrying or archiving a job.
func (s *Server) handleTaskError(ctx context.Context, task *asynq.Task, taskErr error) {
	if task.Type() != TypeExportCSV {
		return
	}
	retried, hasRetried := asynq.GetRetryCount(ctx)
	maxRetry, hasMaxRetry := asynq.GetMaxRetry(ctx)
	terminal := errors.Is(taskErr, asynq.SkipRetry) || errors.Is(taskErr, asynq.RevokeTask)
	if !terminal && (!hasRetried || !hasMaxRetry || retried < maxRetry) {
		return
	}
	var p ExportCSVPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil || p.ExportID <= 0 {
		return
	}
	// Task timeouts cancel ctx. Reporting the final status still needs its own
	// bounded opportunity to reach Postgres after that cancellation.
	statusCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.q.UpdateExportStatus(statusCtx, sqlc.UpdateExportStatusParams{
		ID:       p.ExportID,
		Status:   "error",
		FilePath: pgtype.Text{Valid: false},
	}); err != nil {
		log.Printf("mark export %d failed after terminal task error: %v", p.ExportID, err)
	}
}
