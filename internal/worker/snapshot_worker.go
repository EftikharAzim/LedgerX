package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/hibiken/asynq"
)

type Server struct {
	srv       *asynq.Server
	mux       *asynq.ServeMux
	q         *sqlc.Queries
	redisAddr string
}

func NewServer(redisAddr string, q *sqlc.Queries) *Server {
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{Concurrency: 10, Queues: map[string]int{"default": 10}},
	)
	mux := asynq.NewServeMux()
	ws := &Server{srv: srv, mux: mux, q: q}

	// Register handlers
	mux.HandleFunc(TypeSnapshotAll, ws.handleSnapshotAll)
	mux.HandleFunc(TypeSnapshotAccount, ws.handleSnapshotAccount)

	// 👇 Add this
	mux.HandleFunc(TypeExportCSV, ws.handleExportCSV)

	return ws
}

func (s *Server) Start() error { return s.srv.Start(s.mux) }
func (s *Server) Shutdown()    { s.srv.Shutdown() }
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
	defer client.Close()

	ids, err := s.q.ListAllAccountIDs(ctx)
	if err != nil {
		return err
	}
	// Enqueue one task per account. Use TaskID to dedupe enqueues.
	date, _ := time.Parse("2006-01-02", p.Date)
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
	cutoff := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)

	sum, err := s.q.SumTransactionsUpTo(ctx, sqlc.SumTransactionsUpToParams{
		AccountID: p.AccountID,
		Cutoff:    cutoff,
	})
	if err != nil {
		return err
	}
	if err := s.q.UpsertBalanceSnapshot(ctx, sqlc.UpsertBalanceSnapshotParams{
		AccountID:    p.AccountID,
		AsOfDate:     date, // time.Time (UTC date)
		BalanceMinor: toInt64(sum),
	}); err != nil {
		return err
	}
	return nil
}

// toInt64 converts sqlc's interface{} sum result to int64.
func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int32:
		return int64(t)
	case int:
		return int64(t)
	case []byte:
		if n, err := strconv.ParseInt(string(t), 10, 64); err == nil {
			return n
		}
	case string:
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			return n
		}
	default:
		return 0
	}
	return 0
}

func snapshotKey(accountID int64, date time.Time) string {
	return fmt.Sprintf("snapshot:%d:%s", accountID, date.Format("2006-01-02"))
}
