package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type testOutboxEvent struct {
	id      int64
	kind    string
	payload []byte
}

type testOutboxPool struct {
	events      []testOutboxEvent
	processed   map[int64]bool
	commitError error
}

func (p *testOutboxPool) Begin(context.Context) (pgx.Tx, error) {
	return &testOutboxTx{pool: p}, nil
}

type testOutboxTx struct {
	pgx.Tx
	pool   *testOutboxPool
	marked []int64
}

func (tx *testOutboxTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	var events []testOutboxEvent
	for _, event := range tx.pool.events {
		if !tx.pool.processed[event.id] {
			events = append(events, event)
		}
	}
	return &testOutboxRows{events: events}, nil
}

func (tx *testOutboxTx) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	tx.marked = append(tx.marked, args[0].([]int64)...)
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (tx *testOutboxTx) Commit(context.Context) error {
	if tx.pool.commitError != nil {
		err := tx.pool.commitError
		tx.pool.commitError = nil
		return err
	}
	for _, id := range tx.marked {
		tx.pool.processed[id] = true
	}
	return nil
}

func (tx *testOutboxTx) Rollback(context.Context) error { return nil }

type testOutboxRows struct {
	pgx.Rows
	events []testOutboxEvent
	next   int
}

func (r *testOutboxRows) Next() bool { return r.next < len(r.events) }
func (r *testOutboxRows) Close()     {}
func (r *testOutboxRows) Err() error { return nil }
func (r *testOutboxRows) Scan(dest ...any) error {
	event := r.events[r.next]
	*(dest[0].(*int64)) = event.id
	*(dest[1].(*string)) = event.kind
	*(dest[2].(*[]byte)) = event.payload
	r.next++
	return nil
}

type testEventStream struct {
	err     error
	entries []*redis.XAddArgs
}

func (s *testEventStream) XAdd(ctx context.Context, args *redis.XAddArgs) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx)
	if s.err != nil {
		cmd.SetErr(s.err)
		return cmd
	}
	s.entries = append(s.entries, args)
	cmd.SetVal(fmt.Sprintf("%d-0", len(s.entries)))
	return cmd
}

func TestOutboxRetriesFailedStreamAppend(t *testing.T) {
	pool := &testOutboxPool{
		events:    []testOutboxEvent{{id: 42, kind: "TransactionCreated", payload: []byte(`{"postings":[]}`)}},
		processed: make(map[int64]bool),
	}
	stream := &testEventStream{err: errors.New("redis unavailable")}
	w := &OutboxWorker{pool: pool, rdb: stream, log: zap.NewNop()}
	w.ProcessBatch(context.Background())
	if pool.processed[42] {
		t.Fatal("failed append must leave the PostgreSQL event unprocessed")
	}
	stream.err = nil
	w.ProcessBatch(context.Background())
	if !pool.processed[42] || len(stream.entries) != 1 {
		t.Fatalf("retry did not append and mark event: processed=%v entries=%d", pool.processed, len(stream.entries))
	}
}

func TestOutboxKeepsStableIDAfterCommitFailure(t *testing.T) {
	pool := &testOutboxPool{
		events:      []testOutboxEvent{{id: 42, kind: "TransactionCreated", payload: []byte(`{"postings":[]}`)}},
		processed:   make(map[int64]bool),
		commitError: errors.New("connection lost before commit"),
	}
	stream := &testEventStream{}
	w := &OutboxWorker{pool: pool, rdb: stream, log: zap.NewNop()}
	w.ProcessBatch(context.Background())
	if pool.processed[42] {
		t.Fatal("failed PostgreSQL commit must remain retryable")
	}
	w.ProcessBatch(context.Background())
	if len(stream.entries) != 2 || !pool.processed[42] {
		t.Fatalf("expected replay after commit failure, got %d stream entries", len(stream.entries))
	}
	for _, entry := range stream.entries {
		values := entry.Values.(map[string]any)
		if entry.Stream != OutboxStream || values["outbox_id"] != int64(42) || values["event_type"] != "TransactionCreated" {
			t.Fatalf("replayed event lost its stable identity: %+v", entry)
		}
	}
}

func TestOutboxStreamRetainsEventsForOfflineConsumer(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer func() { _ = rdb.Close() }()
	id := time.Now().UnixNano()
	pool := &testOutboxPool{
		events:    []testOutboxEvent{{id: id, kind: "TransactionCreated", payload: []byte(`{"postings":[]}`)}},
		processed: make(map[int64]bool),
	}
	w := &OutboxWorker{pool: pool, rdb: rdb, log: zap.NewNop()}
	// There is no consumer group when the event is appended and marked.
	w.ProcessBatch(ctx)
	if !pool.processed[id] {
		t.Fatal("stream append failed")
	}
	group := fmt.Sprintf("offline-review-%d", id)
	if err := rdb.XGroupCreate(ctx, OutboxStream, group, "0").Err(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rdb.XGroupDestroy(context.Background(), OutboxStream, group).Err() }()
	for {
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group: group, Consumer: "reconnected", Streams: []string{OutboxStream, ">"}, Count: 100, Block: -1,
		}).Result()
		if err != nil {
			t.Fatalf("offline consumer could not replay retained event: %v", err)
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				if message.Values["outbox_id"] != strconv.FormatInt(id, 10) {
					continue
				}
				defer func() { _ = rdb.XDel(context.Background(), OutboxStream, message.ID).Err() }()
				if err := rdb.XAck(ctx, OutboxStream, group, message.ID).Err(); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
}
