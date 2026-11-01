package worker

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// PrivacyWorker retries file and event cleanup after account deletion. Database
// claims survive restart and permit multiple workers without losing a request.
type PrivacyWorker struct {
	pool  *pgxpool.Pool
	rdb   *redis.Client
	store storage.Storage
	log   *zap.Logger
}

// NewPrivacyWorker connects the durable cleanup queue to the configured stores.
func NewPrivacyWorker(pool *pgxpool.Pool, rdb *redis.Client, store storage.Storage, log *zap.Logger) *PrivacyWorker {
	return &PrivacyWorker{pool: pool, rdb: rdb, store: store, log: log}
}

// Start performs bounded batches until shutdown; failures remain queued.
func (w *PrivacyWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.ProcessBatch(ctx); err != nil {
				w.log.Error("account cleanup will retry", zap.Error(err))
			}
		}
	}
}

// ProcessBatch deletes up to 25 objects and scans up to 500 retained events.
func (w *PrivacyWorker) ProcessBatch(ctx context.Context) error {
	// An object-store outage must not block independent retained-event cleanup.
	return errors.Join(w.cleanFiles(ctx), w.cleanEvents(ctx))
}

func (w *PrivacyWorker) cleanFiles(ctx context.Context) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, "SELECT file_key FROM export_deletions ORDER BY created_at, file_key LIMIT 25 FOR UPDATE SKIP LOCKED")
	if err != nil {
		return err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	var failures error
	for _, key := range keys {
		if err := w.store.Delete(ctx, key); err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		if _, err := tx.Exec(ctx, "DELETE FROM export_deletions WHERE file_key=$1", key); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return failures
}

func (w *PrivacyWorker) cleanEvents(ctx context.Context) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var uid int64
	var cursor string
	err = tx.QueryRow(ctx, "SELECT user_id, stream_cursor FROM privacy_deletions ORDER BY created_at, user_id LIMIT 1 FOR UPDATE SKIP LOCKED").Scan(&uid, &cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	events, err := w.rdb.XRangeN(ctx, OutboxStream, cursor, "+", 500).Result()
	if err != nil {
		return err
	}
	for _, event := range events {
		payload, ok := event.Values["payload"].(string)
		if !ok {
			continue
		}
		var owner struct {
			UserID int64 `json:"user_id"`
		}
		if json.Unmarshal([]byte(payload), &owner) == nil && owner.UserID == uid {
			if err := w.rdb.XDel(ctx, OutboxStream, event.ID).Err(); err != nil {
				return err
			}
		}
	}
	if len(events) == 500 {
		_, err = tx.Exec(ctx, "UPDATE privacy_deletions SET stream_cursor=$2 WHERE user_id=$1", uid, "("+events[len(events)-1].ID)
	} else {
		_, err = tx.Exec(ctx, "DELETE FROM privacy_deletions WHERE user_id=$1", uid)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
