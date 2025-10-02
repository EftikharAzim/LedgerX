package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency.",
			Buckets: prometheus.DefBuckets, // good default buckets
		},
		[]string{"method", "route"},
	)
)

// Register collects metrics in the default registry.
// Call this once on startup.
func Register() {
	prometheus.MustRegister(httpRequestsTotal, httpRequestDuration)
}

// MetricsHandler returns the /metrics handler.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// Middleware instruments each request.
// Use it after route patterns are set so chi's route pattern is available.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Capture status code
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unknown"
			}
			method := r.Method
			status := strconv.Itoa(ww.Status())

			httpRequestsTotal.WithLabelValues(method, route, status).Inc()
			httpRequestDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
		})
	}
}
