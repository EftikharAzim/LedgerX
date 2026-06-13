package worker

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type OutboxWorker struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
	log  *zap.Logger
}

func NewOutboxWorker(pool *pgxpool.Pool, rdb *redis.Client, log *zap.Logger) *OutboxWorker {
	return &OutboxWorker{pool: pool, rdb: rdb, log: log}
}

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
// a crash after publish but before commit republishes the event.
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
		channel := "events:" + e.eventType
		if err := w.rdb.Publish(ctx, channel, e.payload).Err(); err != nil {
			w.log.Error("failed to publish event", zap.Int64("id", e.id), zap.Error(err))
			continue
		}
		w.log.Info("published event", zap.String("channel", channel), zap.Int64("id", e.id))
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
