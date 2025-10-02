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
			w.processBatch(ctx)
		}
	}
}

func (w *OutboxWorker) processBatch(ctx context.Context) {
	rows, err := w.pool.Query(ctx, `
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
	defer rows.Close()

	for rows.Next() {
		var id int64
		var eventType string
		var payload []byte
		if err := rows.Scan(&id, &eventType, &payload); err != nil {
			continue
		}

		// Publish to Redis
		channel := "events:" + eventType
		if err := w.rdb.Publish(ctx, channel, payload).Err(); err != nil {
			w.log.Error("failed to publish event", zap.Error(err))
			continue
		}
		w.log.Info("published event", zap.String("channel", channel), zap.ByteString("payload", payload))

		// Mark processed
		_, err := w.pool.Exec(ctx, `UPDATE outbox SET processed_at = now() WHERE id = $1`, id)
		if err != nil {
			w.log.Error("failed to mark event processed", zap.Error(err))
		}
	}
}
