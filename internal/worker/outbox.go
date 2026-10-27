package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// OutboxStream retains events while consumers are offline. Consumers dedupe
// using outbox_id; a publisher crash before the DB commit can append twice.
const OutboxStream = "events:ledger"

type outboxPool interface {
	Begin(context.Context) (pgx.Tx, error)
}

type outboxStream interface {
	XAdd(context.Context, *redis.XAddArgs) *redis.StringCmd
}

// OutboxWorker copies committed PostgreSQL events into the retained Redis stream.
type OutboxWorker struct {
	pool outboxPool
	rdb  outboxStream
	log  *zap.Logger
}

// NewOutboxWorker builds a publisher using the application's database and Redis clients.
func NewOutboxWorker(pool *pgxpool.Pool, rdb *redis.Client, log *zap.Logger) *OutboxWorker {
	return &OutboxWorker{pool: pool, rdb: rdb, log: log}
}

// Start processes an outbox batch every five seconds until the context is canceled.
func (w *OutboxWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ProcessBatch(ctx)
		}
	}
}

// ProcessBatch claims a batch inside one DB transaction so FOR UPDATE
// SKIP LOCKED actually holds the row locks until the batch commits —
// concurrent workers each get disjoint batches. Delivery is at-least-once:
// a crash after appending to the stream but before commit appends the event
// again with the same outbox_id. A consumer's presence never affects delivery.
func (w *OutboxWorker) ProcessBatch(ctx context.Context) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		w.log.Error("outbox begin failed", zap.Error(err))
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, event_type, payload
		FROM outbox
		WHERE processed_at IS NULL
		ORDER BY created_at
		LIMIT 50
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		w.log.Error("query outbox failed", zap.Error(err))
		return
	}

	type event struct {
		id        int64
		eventType string
		payload   []byte
	}
	var batch []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.id, &e.eventType, &e.payload); err != nil {
			rows.Close()
			w.log.Error("scan outbox row failed", zap.Error(err))
			return
		}
		batch = append(batch, e)
	}
	rows.Close()
	if rows.Err() != nil {
		w.log.Error("outbox rows error", zap.Error(rows.Err()))
		return
	}

	var processed []int64
	for _, e := range batch {
		if err := w.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: OutboxStream,
			Values: map[string]any{
				"outbox_id":  e.id,
				"event_type": e.eventType,
				"payload":    string(e.payload),
			},
		}).Err(); err != nil {
			w.log.Error("failed to append event", zap.Int64("id", e.id), zap.Error(err))
			continue
		}
		w.log.Info("appended event", zap.String("stream", OutboxStream), zap.Int64("id", e.id))
		processed = append(processed, e.id)
	}

	if len(processed) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE outbox SET processed_at = now() WHERE id = ANY($1)`, processed); err != nil {
			w.log.Error("failed to mark events processed", zap.Error(err))
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		w.log.Error("outbox commit failed", zap.Error(err))
	}
}
