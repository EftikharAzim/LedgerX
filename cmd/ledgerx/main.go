package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/EftikharAzim/ledgerx/internal/observability"
	repo "github.com/EftikharAzim/ledgerx/internal/repo"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	service "github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/EftikharAzim/ledgerx/internal/storage"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/EftikharAzim/ledgerx/internal/worker"
	"github.com/joho/godotenv"
)

func main() {
	// ---- Env + Context ----
	_ = godotenv.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	env := getenv("APP_ENV", "dev")
	addr := getenv("HTTP_ADDR", ":8080")
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")

	// ---- Logger ----
	log := observability.MustLogger(env)
	defer func() {
		_ = log.Sync()
	}()

	// ---- Auth ----
	secret := os.Getenv("JWT_SIGNING_KEY")
	if secret == "" {
		if env != "dev" {
			log.Fatal("JWT_SIGNING_KEY is required outside dev")
		}
		secret = "dev-only-insecure-secret"
		log.Warn("JWT_SIGNING_KEY not set; using insecure dev secret")
	}
	if err := service.InitAuth(secret); err != nil {
		log.Fatal("auth init", zap.Error(err))
	}

	// ---- DB ----
	pool := repo.MustOpenPool(ctx)
	defer pool.Close()
	q := sqlc.New(pool)

	// ---- Redis ----
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})

	// ---- Export storage (local disk or S3 via EXPORT_STORAGE) ----
	store, err := storage.FromEnv(ctx)
	if err != nil {
		log.Fatal("storage init", zap.Error(err))
	}

	// ---- Workers ----
	// Runtime errors from the worker/scheduler flow into errCh so the main
	// goroutine can shut everything down through the normal path (a Fatal
	// inside a goroutine would skip the deferred cleanup).
	errCh := make(chan error, 2)

	wrk := worker.NewServer(redisAddr, q, store)
	go func() {
		if err := wrk.Start(); err != nil {
			errCh <- err
		}
	}()
	defer wrk.Shutdown()

	sch := worker.NewScheduler(redisAddr)
	go func() {
		if err := sch.Start(); err != nil {
			errCh <- err
		}
	}()
	defer sch.Shutdown()

	outboxWorker := worker.NewOutboxWorker(pool, rdb, log)
	go outboxWorker.Start(ctx)

	// ---- Observability ----
	observability.Register()
	observability.RegisterDB()

	// ---- HTTP Router ----
	r := httptransport.NewRouter()
	r.Use(observability.Middleware())

	httptransport.HealthRoutes(r, pool, rdb)
	r.Handle("/metrics", observability.MetricsHandler())

	// Unprotected routes
	admin := httptransport.NewAdminAPI(redisAddr)
	admin.Routes(r)

	// Auth endpoints get a per-IP limiter to slow credential stuffing.
	authLimiter := httptransport.NewRateLimiter(rdb,
		envInt("AUTH_RATE_LIMIT_BURST", 10),
		envInt("AUTH_RATE_LIMIT_RPS", 1))
	r.Group(func(rt chi.Router) {
		rt.Use(authLimiter.IPMiddleware)
		auth := httptransport.NewAuthAPI(q)
		auth.Routes(rt)
	})

	// Protected routes under /v1
	rps := envInt("API_RATE_LIMIT_RPS", 1)
	burst := envInt("API_RATE_LIMIT_BURST", rps*10)
	limiter := httptransport.NewRateLimiter(rdb, burst, rps)

	r.Route("/v1", func(rt chi.Router) {
		rt.Use(httptransport.AuthMiddleware)
		rt.Use(limiter.Middleware)

		users := httptransport.NewUsersAPI(q)
		users.Routes(rt)

		accounts := httptransport.NewAccountsAPI(q)
		accounts.Routes(rt)

		summarySvc := service.NewSummaryService(q, rdb)
		txSvc := service.NewTransactionService(pool, q, summarySvc)
		tx := httptransport.NewTransactionsAPI(q, txSvc)
		tx.Routes(rt)

		summary := httptransport.NewSummaryAPI(q, summarySvc)
		summary.Routes(rt)

		exp := httptransport.NewExportsAPI(q, redisAddr, store)
		exp.Routes(rt)

		balSvc := service.NewBalanceService(q)
		balance := httptransport.NewBalanceAPI(q, balSvc)
		balance.Routes(rt)
	})

	// ---- Start HTTP Server ----
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("starting api",
			zap.String("addr", addr),
			zap.String("env", env),
			zap.String("redis", redisAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// ---- Wait for shutdown signal or fatal component error ----
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		log.Error("component failed, shutting down", zap.Error(err))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", zap.Error(err))
	}
	// Worker/scheduler shutdown + pool close run via defers above.
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
