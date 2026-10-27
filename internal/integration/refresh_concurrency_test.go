package integration

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestConcurrentRefreshRejectsReuseAndRevokesSuccessor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	uid, _ := newUserWithAccount(t, "USD")
	raw, hash, err := service.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertRefreshToken(ctx, sqlc.InsertRefreshTokenParams{
		UserID: uid, TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	lock, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Rollback(context.Background()) }()
	if _, err := lock.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, uid); err != nil {
		t.Fatal(err)
	}
	api := httptransport.NewAuthAPI(pool, time.Hour)
	codes := make(chan int, 2)
	for range 2 {
		go func() {
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/auth/refresh", bytes.NewBufferString(`{"refresh_token":"`+raw+`"}`))
			rec := httptest.NewRecorder()
			api.Refresh(rec, req)
			codes <- rec.Code
		}()
	}
	// Both handlers identify the token owner before waiting for the shared
	// user lock. After release, each must reread state inside its transaction.
	for {
		var blocked int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
WHERE datname=current_database() AND wait_event_type='Lock'
AND query LIKE '-- name: LockAuthUser%'`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked == 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("both rotations did not reach the locked row")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	counts := make(map[int]int)
	for range 2 {
		counts[<-codes]++
	}
	if counts[http.StatusOK] != 1 || counts[http.StatusUnauthorized] != 1 {
		t.Fatalf("want one rotation and one reuse rejection, got %v", counts)
	}
	var total, active int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE revoked_at IS NULL)
FROM refresh_tokens WHERE user_id=$1`, uid).Scan(&total, &active); err != nil {
		t.Fatal(err)
	}
	if total != 2 || active != 0 {
		t.Fatalf("want one successor, revoked after reuse; total=%d active=%d", total, active)
	}
}

func TestRefreshRotationRollsBackConsumptionOnInsertFailure(t *testing.T) {
	ctx := context.Background()
	uid, _ := newUserWithAccount(t, "USD")
	_, hash, err := service.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertRefreshToken(ctx, sqlc.InsertRefreshTokenParams{
		UserID: uid, TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	_, err = q.RotateRefreshToken(ctx, sqlc.RotateRefreshTokenParams{
		OldTokenHash: hash, NewTokenHash: hash, ExpiresAt: time.Now().Add(time.Hour),
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("expected successor uniqueness failure, got %v", err)
	}
	old, err := q.GetRefreshToken(ctx, hash)
	if err != nil || old.RevokedAt.Valid {
		t.Fatalf("failed insertion must leave the original token active: %+v, %v", old, err)
	}
}

func TestReuseRevocationWaitsForDescendantRotation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	uid, _ := newUserWithAccount(t, "USD")
	oldRaw, oldHash, err := service.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	_, currentHash, err := service.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	_, descendantHash, err := service.NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour)
	if _, err := q.InsertRefreshToken(ctx, sqlc.InsertRefreshTokenParams{
		UserID: uid, TokenHash: oldHash, ExpiresAt: expires,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RotateRefreshToken(ctx, sqlc.RotateRefreshTokenParams{
		OldTokenHash: oldHash, NewTokenHash: currentHash, ExpiresAt: expires,
	}); err != nil {
		t.Fatal(err)
	}
	// Pause a descendant rotation after insertion but before commit. Without
	// the shared user lock, revoke-all starts with a snapshot that excludes
	// this descendant and waits on the consumed parent row instead.
	rotation, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rotation.Rollback(context.Background()) }()
	qtx := q.WithTx(rotation)
	if _, err := qtx.LockAuthUser(ctx, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := qtx.RotateRefreshToken(ctx, sqlc.RotateRefreshTokenParams{
		OldTokenHash: currentHash, NewTokenHash: descendantHash, ExpiresAt: expires,
	}); err != nil {
		t.Fatal(err)
	}
	api := httptransport.NewAuthAPI(pool, time.Hour)
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/auth/refresh", bytes.NewBufferString(`{"refresh_token":"`+oldRaw+`"}`))
		rec := httptest.NewRecorder()
		api.Refresh(rec, req)
		response <- rec
	}()
	for {
		var waiting int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
WHERE datname=current_database() AND wait_event_type='Lock'
AND query LIKE '-- name: LockAuthUser%'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("reuse revocation did not serialize on the descendant's user lock")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := rotation.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rec := <-response
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused ancestor must be rejected, got %d: %s", rec.Code, rec.Body.String())
	}
	descendant, err := q.GetRefreshToken(ctx, descendantHash)
	if err != nil || !descendant.RevokedAt.Valid {
		t.Fatalf("descendant escaped reuse revocation: %+v, %v", descendant, err)
	}
	var active int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens WHERE user_id=$1 AND revoked_at IS NULL`, uid).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("reuse revocation left %d active tokens", active)
	}
}
