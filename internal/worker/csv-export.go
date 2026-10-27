package worker

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) handleExportCSV(ctx context.Context, t *asynq.Task) error {
	var p ExportCSVPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("invalid export payload: %v: %w", err, asynq.SkipRetry)
	}
	month, err := time.Parse("2006-01-02", p.Month)
	if err != nil {
		return fmt.Errorf("invalid export month: %v: %w", err, asynq.SkipRetry)
	}

	rows, err := s.q.ListPostingsForMonth(ctx, sqlc.ListPostingsForMonthParams{
		UserID:  p.UserID,
		Column2: month,
	})
	if err != nil {
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
		return err
	}

	key := fmt.Sprintf("export_%d.csv", p.ExportID)
	if err := s.store.Save(ctx, key, bytes.NewReader(buf.Bytes())); err != nil {
		return err
	}

	return s.q.UpdateExportStatus(ctx, sqlc.UpdateExportStatusParams{
		ID:       p.ExportID,
		Status:   "done",
		FilePath: pgtype.Text{String: key, Valid: true},
	})
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
