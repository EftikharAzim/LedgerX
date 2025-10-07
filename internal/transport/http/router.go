package httptransport

import (
    "context"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/redis/go-redis/v9"
)

func NewRouter() *chi.Mux {
    r := chi.NewRouter()
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Recoverer)
    r.Use(middleware.Timeout(10 * time.Second))
    // Allow browser clients during development and simple deployments
    r.Use(CORSMiddleware)
    return r
}

func HealthRoutes(r *chi.Mux, pool *pgxpool.Pool, rdb *redis.Client) {
    r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })
    r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
        ctx, cancel := context.WithTimeout(context.Background(), time.Second)
        defer cancel()
        dbOk := pool.Ping(ctx) == nil
        redisOk := rdb.Ping(ctx).Err() == nil
        if dbOk && redisOk {
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write([]byte("ready"))
            return
        }
        w.WriteHeader(http.StatusServiceUnavailable)
        _, _ = w.Write([]byte("not ready"))
    })
}
