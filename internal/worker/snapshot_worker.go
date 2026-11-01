package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/ai"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/storage"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server processes Asynq jobs for exports, snapshots, and category suggestions.
type Server struct {
	pool         *pgxpool.Pool
	srv          *asynq.Server
	mux          *asynq.ServeMux
	q            sqlc.Querier
	store        storage.Storage
	categorizer  ai.Categorizer
	redisAddr    string
	streamExport func(context.Context, sqlc.ListPostingsForExportParams, func(sqlc.ListPostingsForExportRow) error) error
}

// NewServer wires job handlers and terminal export error reporting.
func NewServer(redisAddr string, pool *pgxpool.Pool, store storage.Storage, categorizer ai.Categorizer, concurrency int) *Server {
	if concurrency < 1 || concurrency > 100 {
		panic("WORKER_CONCURRENCY must be between 1 and 100")
	}
	q := sqlc.New(pool)
	mux := asynq.NewServeMux()
	ws := &Server{pool: pool, mux: mux, q: q, store: store, categorizer: categorizer, redisAddr: redisAddr}
	ws.streamExport = q.StreamExportPostings
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency:  concurrency,
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
	err = s.q.UpsertBalanceSnapshot(ctx, sqlc.UpsertBalanceSnapshotParams{
		AccountID:    p.AccountID,
		AsOfDate:     date,
		BalanceMinor: sum,
	})
	if err != nil {
		if _, lookupErr := s.q.GetAccount(ctx, p.AccountID); errors.Is(lookupErr, pgx.ErrNoRows) {
			return nil
		}
	}
	return err
}

func snapshotKey(accountID int64, date time.Time) string {
	return fmt.Sprintf("snapshot:%d:%s", accountID, date.Format("2006-01-02"))
}
