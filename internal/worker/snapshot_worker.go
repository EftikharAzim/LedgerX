package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/ai"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/storage"
	"github.com/hibiken/asynq"
)

// Server processes Asynq jobs for exports, snapshots, and category suggestions.
type Server struct {
	srv         *asynq.Server
	mux         *asynq.ServeMux
	q           sqlc.Querier
	store       storage.Storage
	categorizer ai.Categorizer
	redisAddr   string
}

// NewServer wires job handlers and terminal export error reporting.
func NewServer(redisAddr string, q *sqlc.Queries, store storage.Storage, categorizer ai.Categorizer) *Server {
	mux := asynq.NewServeMux()
	ws := &Server{mux: mux, q: q, store: store, categorizer: categorizer, redisAddr: redisAddr}
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency:  10,
			Queues:       map[string]int{"default": 10},
			ErrorHandler: asynq.ErrorHandlerFunc(ws.handleTaskError),
		},
	)
	ws.srv = srv

	mux.HandleFunc(TypeSnapshotAll, ws.handleSnapshotAll)
	mux.HandleFunc(TypeSnapshotAccount, ws.handleSnapshotAccount)
	mux.HandleFunc(TypeExportCSV, ws.handleExportCSV)
	mux.HandleFunc(TypeCategorizeTx, ws.handleCategorizeTx)

	return ws
}

// Start begins consuming queued tasks.
func (s *Server) Start() error { return s.srv.Start(s.mux) }

// Shutdown drains active jobs and stops task processing.
func (s *Server) Shutdown() { s.srv.Shutdown() }
func (s *Server) newClient() *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{Addr: s.redisAddr})
}

func (s *Server) handleSnapshotAll(ctx context.Context, t *asynq.Task) error {
	var p SnapshotAllPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	// If date not provided, use yesterday in UTC.
	if p.Date == "" {
		p.Date = time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	}
	client := s.newClient()
	defer func() {
		_ = client.Close()
	}()

	ids, err := s.q.ListAllAccountIDs(ctx)
	if err != nil {
		return err
	}
	// Enqueue one task per account. Use TaskID to dedupe enqueues.
	date, err := time.Parse("2006-01-02", p.Date)
	if err != nil {
		return err
	}
	for _, id := range ids {
		task := NewTaskSnapshotAccount(SnapshotAccountPayload{
			AccountID: id,
			Date:      p.Date,
		})
		_, _ = client.EnqueueContext(ctx, task, asynq.Queue("default"), asynq.TaskID(snapshotKey(id, date)))
	}
	return nil
}

func (s *Server) handleSnapshotAccount(ctx context.Context, t *asynq.Task) error {
	var p SnapshotAccountPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return err
	}
	date, err := time.Parse("2006-01-02", p.Date)
	if err != nil {
		return err
	}
	// Cutoff on postings.created_at: matches BalanceService.CurrentBalance,
	// so backdated entries (old occurred_at, new created_at) are never lost.
	cutoff := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)

	sum, err := s.q.SumPostingsUpTo(ctx, sqlc.SumPostingsUpToParams{
		AccountID: p.AccountID,
		Cutoff:    cutoff,
	})
	if err != nil {
		return err
	}
	return s.q.UpsertBalanceSnapshot(ctx, sqlc.UpsertBalanceSnapshotParams{
		AccountID:    p.AccountID,
		AsOfDate:     date,
		BalanceMinor: sum,
	})
}

func snapshotKey(accountID int64, date time.Time) string {
	return fmt.Sprintf("snapshot:%d:%s", accountID, date.Format("2006-01-02"))
}
