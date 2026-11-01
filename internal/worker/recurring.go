package worker

import (
	"context"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/service"
	"go.uber.org/zap"
)

// RecurringWorker prepares pending occurrences while the application is running.
type RecurringWorker struct {
	svc *service.ScheduleService
	log *zap.Logger
}

// NewRecurringWorker shares idempotent planning with the web workflow.
func NewRecurringWorker(s *service.ScheduleService, log *zap.Logger) *RecurringWorker {
	return &RecurringWorker{s, log}
}

// Start refreshes due schedules on startup and every minute, never posting money.
func (w *RecurringWorker) Start(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
		err := w.svc.SyncDue(bounded, time.Now())
		cancel()
		if err != nil && ctx.Err() == nil {
			w.log.Warn("recurring plan refresh failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
