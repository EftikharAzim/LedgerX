package repo

import (
	"context"
	"os"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/observability"
	"github.com/jackc/pgx/v5/pgxpool"
)

func MustOpenPool(ctx context.Context) *pgxpool.Pool {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		// Fallback logic
		if os.Getenv("APP_ENV") == "docker" {
			url = os.Getenv("DATABASE_URL_DOCKER")
		} else {
			url = os.Getenv("DATABASE_URL_LOCAL")
		}
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		panic(err)
	}

	// Attach our Prometheus tracer
	cfg.ConnConfig.Tracer = observability.NewPGXTracer()

	cfg.MaxConns = 10
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 10 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		panic(err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		panic(err)
	}
	return pool
}
