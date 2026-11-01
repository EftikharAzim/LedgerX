package observability

import (
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
)

type queueCollector struct {
	inspector                 *asynq.Inspector
	tasks, latency, reachable *prometheus.Desc
}

// RegisterQueue exposes counts and age without exposing task payloads.
func RegisterQueue(addr string) func() error {
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: addr, DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second})
	prometheus.MustRegister(&queueCollector{
		inspector: inspector,
		tasks:     prometheus.NewDesc("ledgerx_queue_tasks", "Current default-queue jobs by state.", []string{"state"}, nil),
		latency:   prometheus.NewDesc("ledgerx_queue_oldest_pending_seconds", "Age of the oldest pending job.", nil, nil),
		reachable: prometheus.NewDesc("ledgerx_queue_reachable", "Whether queue statistics are available.", nil, nil),
	})
	return inspector.Close
}

func (c *queueCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.tasks
	ch <- c.latency
	ch <- c.reachable
}

func (c *queueCollector) Collect(ch chan<- prometheus.Metric) {
	info, err := c.inspector.GetQueueInfo("default")
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.reachable, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.reachable, prometheus.GaugeValue, 1)
	ch <- prometheus.MustNewConstMetric(c.latency, prometheus.GaugeValue, info.Latency.Seconds())
	for state, value := range map[string]int{"pending": info.Pending, "active": info.Active, "retry": info.Retry, "scheduled": info.Scheduled, "archived": info.Archived} {
		ch <- prometheus.MustNewConstMetric(c.tasks, prometheus.GaugeValue, float64(value), state)
	}
}
