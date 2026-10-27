package worker

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// CategorizeEnqueuer enqueues categorization jobs. It satisfies the
// service.CategorizeEnqueuer interface, keeping the service layer free of any
// Asynq/Redis dependency.
type CategorizeEnqueuer struct {
	client *asynq.Client
}

// NewCategorizeEnqueuer connects a categorization producer to the Redis queue.
func NewCategorizeEnqueuer(redisAddr string) *CategorizeEnqueuer {
	return &CategorizeEnqueuer{client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})}
}

// EnqueueCategorize schedules categorization for a transaction. The TaskID
// dedupes re-enqueues for the same transaction within asynq's retention.
// Best-effort: a failure to enqueue must not fail the originating request.
func (e *CategorizeEnqueuer) EnqueueCategorize(ctx context.Context, txID, userID int64) {
	task := asynq.NewTask(TypeCategorizeTx, MustJSON(CategorizeTxPayload{
		TransactionID: txID, UserID: userID,
	}))
	_, _ = e.client.EnqueueContext(ctx, task, asynq.TaskID(fmt.Sprintf("categorize:%d", txID)))
}

// Close releases the enqueuer's Redis connections.
func (e *CategorizeEnqueuer) Close() error { return e.client.Close() }
