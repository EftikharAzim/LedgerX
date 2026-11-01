package observability

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterPool exposes connection pressure so pool sizes can follow measurements.
func RegisterPool(pool *pgxpool.Pool) {
	for _, metric := range []struct {
		name, help string
		value      func() float64
	}{
		{"db_pool_acquired_connections", "Connections currently in use.", func() float64 { return float64(pool.Stat().AcquiredConns()) }},
		{"db_pool_idle_connections", "Idle database connections.", func() float64 { return float64(pool.Stat().IdleConns()) }},
		{"db_pool_max_connections", "Configured maximum database connections.", func() float64 { return float64(pool.Stat().MaxConns()) }},
	} {
		prometheus.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: metric.name, Help: metric.help}, metric.value))
	}
	prometheus.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "db_pool_acquire_duration_seconds_total", Help: "Cumulative time acquiring database connections.",
	}, func() float64 { return pool.Stat().AcquireDuration().Seconds() }))
	prometheus.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "db_pool_empty_acquire_total", Help: "Acquisitions that waited for an available connection.",
	}, func() float64 { return float64(pool.Stat().EmptyAcquireCount()) }))
	prometheus.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "db_pool_canceled_acquire_total", Help: "Canceled connection acquisitions.",
	}, func() float64 { return float64(pool.Stat().CanceledAcquireCount()) }))
}
