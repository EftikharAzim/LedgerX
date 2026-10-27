// internal/worker/scheduler.go

package worker

import (
	"time"

	"github.com/hibiken/asynq"
)

// Scheduler enqueues the nightly UTC snapshot fan-out.
type Scheduler struct {
	s *asynq.Scheduler
}

// NewScheduler connects a UTC scheduler to the Redis job queue.
func NewScheduler(redisAddr string) *Scheduler {
	s := asynq.NewScheduler(asynq.RedisClientOpt{Addr: redisAddr}, &asynq.SchedulerOpts{
		Location: time.UTC,
	})
	return &Scheduler{s: s}
}

// Start registers the daily 00:05 UTC snapshot job and starts scheduling.
func (sch *Scheduler) Start() error {
	// Run at 00:05 UTC daily with an EMPTY payload (handler will fill "yesterday")
	_, err := sch.s.Register("5 0 * * *", asynq.NewTask(TypeSnapshotAll, MustJSON(SnapshotAllPayload{})))
	if err != nil {
		return err
	}
	return sch.s.Start()
}

// Shutdown stops the scheduler and releases its connections.
func (sch *Scheduler) Shutdown() { sch.s.Shutdown() }
