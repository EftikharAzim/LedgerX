package httptransport

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// The barrier makes both initial lookups see an active token. Transactions
// then serialize on the user lock and reread token state before changing it.
type refreshRaceDB struct {
	mu          sync.Mutex
	userLock    sync.Mutex
	record      sqlc.RefreshToken
	reads       int
	bothRead    chan struct{}
	minted      int
	revokedAll  bool
	rotationErr error
}

type refreshTestRow struct {
	record sqlc.RefreshToken
	err    error
	ready  <-chan struct{}
}

func (r refreshTestRow) Scan(dest ...any) error {
	if r.ready != nil {
		<-r.ready
	}
	if r.err != nil {
		return r.err
	}
	if len(dest) == 1 {
		*dest[0].(*int64) = r.record.UserID
		return nil
	}
	*dest[0].(*int64) = r.record.ID
	*dest[1].(*int64) = r.record.UserID
	*dest[2].(*string) = r.record.TokenHash
	*dest[3].(*time.Time) = r.record.ExpiresAt
	*dest[4].(*pgtype.Timestamptz) = r.record.RevokedAt
	*dest[5].(*time.Time) = r.record.CreatedAt
	return nil
}

type refreshTestTx struct {
	pgx.Tx
	db     *refreshRaceDB
	locked bool
}

func (db *refreshRaceDB) BeginTx(_ context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	if options.IsoLevel != pgx.ReadCommitted {
		panic("auth transaction must use READ COMMITTED")
	}
	return &refreshTestTx{db: db}, nil
}

func (tx *refreshTestTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	if strings.Contains(query, "LockAuthUser") {
		tx.db.userLock.Lock()
		tx.locked = true
		return refreshTestRow{record: sqlc.RefreshToken{UserID: args[0].(int64)}}
	}
	return tx.db.QueryRow(ctx, query, args...)
}

func (tx *refreshTestTx) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	return tx.db.Exec(ctx, query, args...)
}

func (tx *refreshTestTx) Commit(context.Context) error {
	if tx.locked {
		tx.locked = false
		tx.db.userLock.Unlock()
	}
	return nil
}

func (tx *refreshTestTx) Rollback(ctx context.Context) error { return tx.Commit(ctx) }

func (db *refreshRaceDB) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	db.mu.Lock()
	defer db.mu.Unlock()
	switch {
	case strings.Contains(query, "GetRefreshToken"):
		snapshot := db.record
		if db.bothRead != nil {
			db.reads++
			if db.reads == 2 {
				close(db.bothRead)
			}
		}
		return refreshTestRow{record: snapshot, ready: db.bothRead}
	case strings.Contains(query, "RotateRefreshToken"):
		if db.rotationErr != nil {
			return refreshTestRow{err: db.rotationErr}
		}
		if db.record.RevokedAt.Valid {
			return refreshTestRow{err: pgx.ErrNoRows}
		}
		db.record.RevokedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		db.minted++
		return refreshTestRow{record: sqlc.RefreshToken{
			ID: 2, UserID: db.record.UserID, TokenHash: args[0].(string),
			ExpiresAt: args[1].(time.Time), CreatedAt: time.Now(),
		}}
	default:
		panic("unexpected refresh query: " + query)
	}
}

func (db *refreshRaceDB) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if !strings.Contains(query, "RevokeAllUserRefreshTokens") {
		panic("unexpected refresh exec: " + query)
	}
	db.revokedAll = true
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (*refreshRaceDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected rows query")
}

func newRefreshRaceDB() *refreshRaceDB {
	return &refreshRaceDB{record: sqlc.RefreshToken{
		ID: 1, UserID: 1, TokenHash: service.HashToken("same-token"),
		ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
	}}
}

func refreshRequest(api *AuthAPI) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBufferString(`{"refresh_token":"same-token"}`))
	rec := httptest.NewRecorder()
	api.Refresh(rec, req)
	return rec
}

func TestConcurrentRefreshConsumesTokenOnce(t *testing.T) {
	if err := service.InitAuth("refresh-concurrency-test"); err != nil {
		t.Fatal(err)
	}
	db := newRefreshRaceDB()
	db.bothRead = make(chan struct{})
	api := NewAuthAPI(db, time.Hour)
	codes := make(chan int, 2)
	for range 2 {
		go func() { codes <- refreshRequest(api).Code }()
	}
	counts := make(map[int]int)
	for range 2 {
		counts[<-codes]++
	}
	if counts[http.StatusOK] != 1 || counts[http.StatusUnauthorized] != 1 {
		t.Fatalf("want one successful rotation and one rejected reuse, got %v", counts)
	}
	if db.minted != 1 || !db.revokedAll {
		t.Fatalf("want one successor revoked after reuse; minted=%d revoked=%v", db.minted, db.revokedAll)
	}
}

func TestRefreshStorageFailureDoesNotIssueTokens(t *testing.T) {
	if err := service.InitAuth("refresh-storage-test"); err != nil {
		t.Fatal(err)
	}
	db := newRefreshRaceDB()
	db.rotationErr = errors.New("database unavailable")
	rec := refreshRequest(NewAuthAPI(db, time.Hour))
	if rec.Code != http.StatusInternalServerError || db.minted != 0 || db.record.RevokedAt.Valid {
		t.Fatalf("failed rotation must not issue tokens: status=%d minted=%d revoked=%v", rec.Code, db.minted, db.record.RevokedAt.Valid)
	}
}
