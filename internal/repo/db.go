// Package repo opens the PostgreSQL connection pool used by the application.
package repo

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/observability"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MustOpenPool opens a configured pool and panics if the database is unreachable.
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

	cfg.MaxConns = poolSetting("DB_MAX_CONNS", 10)
	cfg.MinConns = poolSetting("DB_MIN_CONNS", 2)
	if cfg.MaxConns < 1 || cfg.MinConns < 0 || cfg.MinConns > cfg.MaxConns {
		panic("DB_MAX_CONNS must be positive and DB_MIN_CONNS must be between zero and DB_MAX_CONNS")
	}
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

func poolSetting(name string, fallback int32) int32 {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		panic(fmt.Sprintf("invalid %s", name))
	}
	return int32(value)
}
