package observability

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	dbQueryTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "db_queries_total",
			Help: "Total DB queries executed.",
		},
		[]string{"op", "sql", "status"}, // op=SELECT/INSERT/UPDATE/DELETE, sql=hash, status=ok|error
	)

	dbQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "DB query latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"op", "sql"},
	)
)

// RegisterDB adds DB metrics to the registry. Call this inside Register().
func RegisterDB() {
	prometheus.MustRegister(dbQueryTotal, dbQueryDuration)
}

// ---- pgx tracer ----

// pgxTracer implements pgx.Tracer to observe query start/end.
type pgxQueryTracer struct{}

func NewPGXTracer() pgx.QueryTracer { return &pgxQueryTracer{} }

type spanData struct {
	start time.Time
	op    string
	sqlh  string
}

type ctxKey struct{}

// This fires before a query runs
func (t *pgxQueryTracer) TraceQueryStart(
	ctx context.Context,
	_ *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {
	op := opOf(data.SQL)
	sqlh := hashSQL(data.SQL)
	return context.WithValue(ctx, ctxKey{}, spanData{
		start: time.Now(),
		op:    op,
		sqlh:  sqlh,
	})
}

// This fires after a query finishes
func (t *pgxQueryTracer) TraceQueryEnd(
	ctx context.Context,
	_ *pgx.Conn,
	data pgx.TraceQueryEndData,
) {
	v := ctx.Value(ctxKey{})
	s, _ := v.(spanData)
	if s.start.IsZero() {
		return
	}
	dur := time.Since(s.start).Seconds()
	dbQueryDuration.WithLabelValues(s.op, s.sqlh).Observe(dur)

	status := "ok"
	if data.Err != nil {
		status = "error"
	}
	dbQueryTotal.WithLabelValues(s.op, s.sqlh, status).Inc()
}

// Helpers
func opOf(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "OTHER"
	}
	up := strings.ToUpper(sql)
	switch {
	case strings.HasPrefix(up, "SELECT"):
		return "SELECT"
	case strings.HasPrefix(up, "INSERT"):
		return "INSERT"
	case strings.HasPrefix(up, "UPDATE"):
		return "UPDATE"
	case strings.HasPrefix(up, "DELETE"):
		return "DELETE"
	default:
		return "OTHER"
	}
}

func hashSQL(sql string) string {
	sum := sha1.Sum([]byte(sql))
	return hex.EncodeToString(sum[:8])
}
