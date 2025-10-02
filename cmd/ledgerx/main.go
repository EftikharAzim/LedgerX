package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/EftikharAzim/ledgerx/internal/observability"
	repo "github.com/EftikharAzim/ledgerx/internal/repo"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	service "github.com/EftikharAzim/ledgerx/internal/service"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/EftikharAzim/ledgerx/internal/worker"
	"github.com/joho/godotenv"
)

func main() {
	// ---- Env + Context ----
	_ = godotenv.Load()
	ctx := context.Background()
	env := getenv("APP_ENV", "dev")
	addr := getenv("HTTP_ADDR", ":8080")
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")

	// ---- Logger ----
	log := observability.MustLogger(env)
	defer log.Sync()

	// ---- DB ----
	pool := repo.MustOpenPool(ctx)
	defer pool.Close()
	q := sqlc.New(pool)

	// ---- Redis ----
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	// ---- Workers ----
	// async job worker (exports, balance snapshots)
	wrk := worker.NewServer(redisAddr, q)
	go func() {
		if err := wrk.Start(); err != nil {
			log.Fatal("worker server error", zap.Error(err))
		}
	}()
	defer wrk.Shutdown()

	// scheduler (nightly balance snapshot jobs)
	sch := worker.NewScheduler(redisAddr)
	go func() {
		if err := sch.Start(); err != nil {
			log.Fatal("scheduler start error", zap.Error(err))
		}
	}()
	defer sch.Shutdown()

	// outbox publisher (publishes DB events to Redis Pub/Sub)
	outboxWorker := worker.NewOutboxWorker(pool, rdb, log)
	go outboxWorker.Start(ctx)

	// ---- Observability ----
	observability.Register()
	observability.RegisterDB()

	// ---- HTTP Router ----
	r := httptransport.NewRouter()
	r.Use(observability.Middleware())

	// Health & Metrics
	httptransport.HealthRoutes(r)
	r.Handle("/metrics", observability.MetricsHandler())

	// Unprotected routes
	admin := httptransport.NewAdminAPI(redisAddr)
	admin.Routes(r)

	exports := httptransport.NewExportsAPI(q, redisAddr)
	exports.Routes(r)

	auth := httptransport.NewAuthAPI(q)
	auth.Routes(r)

	// Protected routes under /v1
	limiter := httptransport.NewRateLimiter(rdb, 60, time.Second) // 60 req/min

	r.Route("/v1", func(rt chi.Router) {
		rt.Use(httptransport.AuthMiddleware)
		rt.Use(limiter.Middleware)

		users := httptransport.NewUsersAPI(q)
		users.Routes(rt)

		accounts := httptransport.NewAccountsAPI(q)
		accounts.Routes(rt)

		summarySvc := service.NewSummaryService(q, rdb)
		tx := httptransport.NewTransactionsAPI(q, summarySvc)
		tx.Routes(rt)

		summary := httptransport.NewSummaryAPI(summarySvc)
		summary.Routes(rt)

		exp := httptransport.NewExportsAPI(q, redisAddr)
		exp.Routes(rt)

		balSvc := service.NewBalanceService(q)
		balance := httptransport.NewBalanceAPI(balSvc)
		balance.Routes(rt)
	})

	// ---- Start HTTP Server ----
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Info("starting api",
		zap.String("addr", addr),
		zap.String("env", env),
		zap.String("redis", redisAddr))

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("server error", zap.Error(err))
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
