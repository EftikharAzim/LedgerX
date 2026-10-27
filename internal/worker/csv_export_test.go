package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/hibiken/asynq"
)

type testExportQueries struct {
	sqlc.Querier
	mu       sync.Mutex
	statuses []string
	updates  chan string
}

func (q *testExportQueries) ListPostingsForMonth(context.Context, sqlc.ListPostingsForMonthParams) ([]sqlc.ListPostingsForMonthRow, error) {
	return nil, nil
}

func (q *testExportQueries) UpdateExportStatus(ctx context.Context, params sqlc.UpdateExportStatusParams) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	q.statuses = append(q.statuses, params.Status)
	q.mu.Unlock()
	if q.updates != nil {
		q.updates <- params.Status
	}
	return nil
}

type testExportStorage struct {
	remainingFailures int
}

func (s *testExportStorage) Save(context.Context, string, io.Reader) error {
	if s.remainingFailures > 0 {
		s.remainingFailures--
		return errors.New("transient object storage failure")
	}
	return nil
}

func (*testExportStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented in test")
}

func exportTestTask(month string) *asynq.Task {
	return asynq.NewTask(TypeExportCSV, MustJSON(ExportCSVPayload{ExportID: 42, UserID: 7, Month: month}))
}

func TestExportTransientFailureDoesNotBecomeTerminal(t *testing.T) {
	q := &testExportQueries{}
	s := &Server{q: q, store: &testExportStorage{remainingFailures: 1}}
	task := exportTestTask("2026-10-01")
	err := s.handleExportCSV(context.Background(), task)
	if err == nil {
		t.Fatal("expected first storage attempt to fail")
	}
	s.handleTaskError(context.Background(), task, err)
	if len(q.statuses) != 0 {
		t.Fatalf("retryable error must leave status pending, got %v", q.statuses)
	}
	if err := s.handleExportCSV(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(q.statuses) != 1 || q.statuses[0] != "done" {
		t.Fatalf("successful retry should finish pending export, got %v", q.statuses)
	}
}

func TestExportInvalidMonthIsTerminalEvenAfterContextCancellation(t *testing.T) {
	q := &testExportQueries{}
	s := &Server{q: q}
	task := exportTestTask("invalid")
	err := s.handleExportCSV(context.Background(), task)
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("invalid month must skip retries, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.handleTaskError(ctx, task, err)
	if len(q.statuses) != 1 || q.statuses[0] != "error" {
		t.Fatalf("permanent error must report terminal status despite cancelled task, got %v", q.statuses)
	}
}

func TestExportRetryExhaustionReportsTerminalError(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set")
	}
	q := &testExportQueries{updates: make(chan string, 4)}
	s := &Server{q: q, store: &testExportStorage{remainingFailures: 10}}
	queue := fmt.Sprintf("export-review-%d", time.Now().UnixNano())
	srv := asynq.NewServer(asynq.RedisClientOpt{Addr: addr}, asynq.Config{
		Concurrency: 1, Queues: map[string]int{queue: 1},
		TaskCheckInterval: 10 * time.Millisecond, DelayedTaskCheckInterval: 10 * time.Millisecond,
		RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 0 },
		ErrorHandler:   asynq.ErrorHandlerFunc(s.handleTaskError),
		LogLevel:       asynq.ErrorLevel,
	})
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: addr})
	defer func() {
		srv.Shutdown()
		_ = inspector.DeleteQueue(queue, true)
		_ = inspector.Close()
	}()
	if err := srv.Start(asynq.HandlerFunc(s.handleExportCSV)); err != nil {
		t.Fatal(err)
	}
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: addr})
	defer func() { _ = client.Close() }()
	if _, err := client.Enqueue(exportTestTask("2026-10-01"), asynq.Queue(queue), asynq.MaxRetry(1)); err != nil {
		t.Fatal(err)
	}
	select {
	case status := <-q.updates:
		if status != "error" {
			t.Fatalf("expected terminal status after retry exhaustion, got %q", status)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("terminal status never reported")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.statuses) != 1 {
		t.Fatalf("status was updated before retry exhaustion: %v", q.statuses)
	}
}
