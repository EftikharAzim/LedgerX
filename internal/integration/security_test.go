package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/EftikharAzim/ledgerx/internal/storage"
	transport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/EftikharAzim/ledgerx/internal/worker"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func accountSecurityRouter() http.Handler {
	r := transport.NewRouter()
	a := transport.NewAuthAPI(pool, time.Hour)
	a.Routes(r)
	r.Route("/v1", func(r chi.Router) {
		r.Use(transport.AuthMiddleware, transport.RequireCurrentSession(q))
		a.SecurityRoutes(r)
		r.Get("/probe", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	})
	return r
}

func securityRequest(t *testing.T, router http.Handler, method, path, token string, input any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func securityLogin(t *testing.T, router http.Handler, email, password string) map[string]string {
	t.Helper()
	w := securityRequest(t, router, "POST", "/auth/login", "", map[string]string{"email": email, "password": password})
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var data struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"access": data.Access, "refresh": data.Refresh}
}

func TestRecoveryIsSingleUseAndPasswordChangesEndEverySession(t *testing.T) {
	ctx := context.Background()
	uid, _ := newUserWithAccount(t, "USD")
	hash, err := service.HashPassword("original-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET password_hash=$2 WHERE id=$1", uid, hash); err != nil {
		t.Fatal(err)
	}
	u, err := q.GetUserByID(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	router := accountSecurityRouter()
	first := securityLogin(t, router, u.Email, "original-password")
	second := securityLogin(t, router, u.Email, "original-password")
	generate := func(password string) string {
		t.Helper()
		w := securityRequest(t, router, "POST", "/v1/security/recovery-code", first["access"], map[string]string{"current_password": password})
		if w.Code != 200 {
			t.Fatalf("generate code: %d %s", w.Code, w.Body.String())
		}
		var data map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		return data["recovery_code"]
	}
	oldCode := generate("original-password")
	code := generate("original-password")
	if len(code) != 68 || code == oldCode {
		t.Fatal("recovery codes must be fresh and high entropy")
	}
	stored, err := q.GetSecurityState(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RecoveryCodeHash.String == code || stored.RecoveryCodeHash.String != service.HashToken(code) {
		t.Fatal("raw recovery code stored or wrong hash")
	}
	invalid := securityRequest(t, router, "POST", "/auth/recover", "", map[string]string{"email": u.Email, "recovery_code": oldCode, "password": "recovered-password"})
	if invalid.Code != 401 {
		t.Fatalf("replaced code still valid: %d", invalid.Code)
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Go(func() {
			w := securityRequest(t, router, "POST", "/auth/recover", "", map[string]string{"email": u.Email, "recovery_code": code, "password": "recovered-password"})
			codes <- w.Code
		})
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[204] != 1 || counts[401] != 1 {
		t.Fatalf("concurrent recovery outcomes: %v", counts)
	}
	for _, pair := range []map[string]string{first, second} {
		if w := securityRequest(t, router, "GET", "/v1/probe", pair["access"], nil); w.Code != 401 {
			t.Fatalf("old access survived reset: %d", w.Code)
		}
		if w := securityRequest(t, router, "POST", "/auth/refresh", "", map[string]string{"refresh_token": pair["refresh"]}); w.Code != 401 {
			t.Fatalf("old refresh survived reset: %d", w.Code)
		}
	}
	if w := securityRequest(t, router, "POST", "/auth/login", "", map[string]string{"email": u.Email, "password": "original-password"}); w.Code != 401 {
		t.Fatal("old password survived reset")
	}
	active := securityLogin(t, router, u.Email, "recovered-password")
	wrong := securityRequest(t, router, "POST", "/v1/security/password", active["access"], map[string]string{"current_password": "wrong", "password": "changed-password"})
	if wrong.Code != 403 {
		t.Fatalf("wrong current password: %d", wrong.Code)
	}
	if w := securityRequest(t, router, "GET", "/v1/probe", active["access"], nil); w.Code != 204 {
		t.Fatal("wrong password ended valid session")
	}
	changed := securityRequest(t, router, "POST", "/v1/security/password", active["access"], map[string]string{"current_password": "recovered-password", "password": "changed-password"})
	if changed.Code != 204 {
		t.Fatalf("change password: %d %s", changed.Code, changed.Body.String())
	}
	if w := securityRequest(t, router, "GET", "/v1/probe", active["access"], nil); w.Code != 401 {
		t.Fatal("old access survived password change")
	}
	_ = securityLogin(t, router, u.Email, "changed-password")
	stored, err = q.GetSecurityState(ctx, uid)
	if err != nil || stored.AuthVersion != 2 || stored.RecoveryCodeHash.Valid {
		t.Fatalf("reset state: version=%d recovery=%v err=%v", stored.AuthVersion, stored.RecoveryCodeHash.Valid, err)
	}
}

type retryDeleteStorage struct {
	storage.Storage
	fail bool
}

func (s *retryDeleteStorage) Delete(ctx context.Context, key string) error {
	if s.fail {
		return fmt.Errorf("simulated storage outage")
	}
	return s.Storage.Delete(ctx, key)
}

func TestAccountDeletionCascadesAndDurablyRetriesPrivateFileCleanup(t *testing.T) {
	ctx := context.Background()
	uid, account := newUserWithAccount(t, "USD")
	other, _ := newUserWithAccount(t, "USD")
	hash, err := service.HashPassword("delete-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET password_hash=$2 WHERE id=$1", uid, hash); err != nil {
		t.Fatal(err)
	}
	category, err := q.CreateCategory(ctx, sqlc.CreateCategoryParams{UserID: uid, Name: "Deletion test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: account, AmountMinor: -123, Currency: "USD", OccurredAt: time.Now(), CategoryID: category.ID, IdempotencyKey: "delete-test"})
	if err != nil {
		t.Fatal(err)
	}
	var exportID int64
	if err := pool.QueryRow(ctx, "INSERT INTO exports(user_id, report_type, status) VALUES ($1,'all','done') RETURNING id", uid).Scan(&exportID); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("export_%d.csv", exportID)
	if err := store.Save(ctx, key, strings.NewReader("private test CSV")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE exports SET file_path=$2 WHERE id=$1", exportID, key); err != nil {
		t.Fatal(err)
	}
	user, err := q.GetUserByID(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	privateEvent, err := rdb.XAdd(ctx, &redis.XAddArgs{Stream: worker.OutboxStream, Values: map[string]any{"payload": fmt.Sprintf(`{"user_id":%d,"amount_minor":123}`, uid)}}).Result()
	if err != nil {
		t.Fatal(err)
	}
	otherEvent, err := rdb.XAdd(ctx, &redis.XAddArgs{Stream: worker.OutboxStream, Values: map[string]any{"payload": fmt.Sprintf(`{"user_id":%d,"amount_minor":999}`, other)}}).Result()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.XDel(context.Background(), worker.OutboxStream, privateEvent, otherEvent).Err() })
	router := accountSecurityRouter()
	pair := securityLogin(t, router, user.Email, "delete-password")
	for _, body := range []map[string]string{{"current_password": "delete-password", "confirmation": "no"}, {"current_password": "wrong", "confirmation": "DELETE"}} {
		w := securityRequest(t, router, "DELETE", "/v1/security/account", pair["access"], body)
		if w.Code != 403 && w.Code != 422 {
			t.Fatalf("unsafe deletion accepted: %d", w.Code)
		}
		if _, err := q.GetUserByID(ctx, uid); err != nil {
			t.Fatal("rejected deletion changed user", err)
		}
	}
	// A job that is already generating a report must finish before deletion can
	// queue its key. This also covers objects whose 'done' update hasn't landed.
	lease, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Rollback(ctx) }()
	if _, err := lease.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(hashtextextended('ledger-export-user:' || $1::bigint::text,0))", uid); err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result <- securityRequest(t, router, "DELETE", "/v1/security/account", pair["access"], map[string]string{"current_password": "delete-password", "confirmation": "DELETE"})
	}()
	select {
	case <-result:
		t.Fatal("deletion did not wait for the export lease")
	case <-time.After(100 * time.Millisecond):
	}
	if err := lease.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	w := <-result
	if w.Code != 204 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	for _, table := range []string{"users", "accounts", "transactions", "categories", "exports", "idempotency_keys", "refresh_tokens"} {
		column := "user_id"
		if table == "users" {
			column = "id"
		}
		var count int
		// Table/column names above are a fixed test allowlist.
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE "+column+"=$1", uid).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s retained rows: %d %v", table, count, err)
		}
	}
	if _, err := q.GetUserByID(ctx, other); err != nil {
		t.Fatal("other user was deleted", err)
	}
	if w := securityRequest(t, router, "GET", "/v1/probe", pair["access"], nil); w.Code != 401 {
		t.Fatal("deleted account's JWT still works")
	}
	flaky := &retryDeleteStorage{Storage: store, fail: true}
	cleanup := worker.NewPrivacyWorker(pool, rdb, flaky, zap.NewNop())
	if err := cleanup.ProcessBatch(ctx); err == nil {
		t.Fatal("expected storage outage")
	}
	for range 20 {
		remaining, err := rdb.XRange(ctx, worker.OutboxStream, privateEvent, privateEvent).Result()
		if err != nil {
			t.Fatal(err)
		}
		if len(remaining) == 0 {
			break
		}
		_ = cleanup.ProcessBatch(ctx) // Files still fail; stream cleanup must progress.
	}
	remaining, err := rdb.XRange(ctx, worker.OutboxStream, privateEvent, privateEvent).Result()
	if err != nil || len(remaining) != 0 {
		t.Fatal("retained private event survived independent cleanup", err)
	}
	kept, err := rdb.XRange(ctx, worker.OutboxStream, otherEvent, otherEvent).Result()
	if err != nil || len(kept) != 1 {
		t.Fatal("another user's retained event was removed", err)
	}
	var queued int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM export_deletions WHERE file_key=$1", key).Scan(&queued); err != nil || queued != 1 {
		t.Fatal("cleanup request lost", err)
	}
	flaky.fail = false
	if err := cleanup.ProcessBatch(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(ctx, key); !os.IsNotExist(err) {
		t.Fatalf("private report remains: %v", err)
	}
	if err := cleanup.ProcessBatch(ctx); err != nil {
		t.Fatal("cleanup must be idempotent", err)
	}
	// Normal append-only protection remains enforced by the existing suite.
}
