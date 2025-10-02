// internal/worker/scheduler.go

package worker

import (
	"time"

	"github.com/hibiken/asynq"
)

type Scheduler struct {
	s *asynq.Scheduler
}

func NewScheduler(redisAddr string) *Scheduler {
	s := asynq.NewScheduler(asynq.RedisClientOpt{Addr: redisAddr}, &asynq.SchedulerOpts{
		Location: time.UTC,
	})
	return &Scheduler{s: s}
}

func (sch *Scheduler) Start() error {
	// Run at 00:05 UTC daily with an EMPTY payload (handler will fill "yesterday")
	_, err := sch.s.Register("5 0 * * *", asynq.NewTask(TypeSnapshotAll, MustJSON(SnapshotAllPayload{})))
	if err != nil {
		return err
	}
	return sch.s.Start()
}

func (sch *Scheduler) Shutdown() { sch.s.Shutdown() }
