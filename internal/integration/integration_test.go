// Package integration exercises the real stack: Postgres (schema, trigger,
// transactionality) and Redis (summary cache, outbox publishing).
//
// Run with:
//
//	TEST_DATABASE_URL=postgres://ledgerx:ledgerx@localhost:5432/ledgerx_test?sslmode=disable \
//	TEST_REDIS_ADDR=localhost:6379 go test ./internal/integration/
//
// Tests are skipped when TEST_DATABASE_URL is unset. The database name must
// contain "test": the suite drops and recreates the public schema.
package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/EftikharAzim/ledgerx/internal/worker"
)

var (
	pool   *pgxpool.Pool
	q      *sqlc.Queries
	rdb    *redis.Client
	txSvc  *service.TransactionService
	balSvc *service.BalanceService
	sumSvc *service.SummaryService
)

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL not set; skipping integration tests")
		os.Exit(0)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		panic(err)
	}
	if !strings.Contains(cfg.ConnConfig.Database, "test") {
		panic("refusing to run: TEST_DATABASE_URL database name must contain 'test' (schema is dropped)")
	}

	ctx := context.Background()
	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		panic(err)
	}
	if err := resetAndMigrate(ctx); err != nil {
		panic(err)
	}

	redisAddr := os.Getenv("TEST_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb = redis.NewClient(&redis.Options{Addr: redisAddr})

	q = sqlc.New(pool)
	sumSvc = service.NewSummaryService(q, rdb)
	txSvc = service.NewTransactionService(pool, q, sumSvc)
	balSvc = service.NewBalanceService(q)
	if err := service.InitAuth("integration-test-secret"); err != nil {
		panic(err)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// resetAndMigrate drops the public schema and replays every up-migration in
// order — the suite always runs against the exact schema in ./migrations.
func resetAndMigrate(ctx context.Context) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		return err
	}
	files, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return errors.New("no migration files found")
	}
	for _, f := range files {
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("migration %s: %w", filepath.Base(f), err)
		}
	}
	return nil
}

var userSeq int

func newUserWithAccount(t *testing.T, currency string) (int64, int64) {
	t.Helper()
	userSeq++
	u, err := q.CreateUser(context.Background(), sqlc.CreateUserParams{
		Email:        fmt.Sprintf("it-%d-%d@example.com", os.Getpid(), userSeq),
		PasswordHash: "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	a, err := q.CreateAccount(context.Background(), sqlc.CreateAccountParams{
		UserID: u.ID, Name: fmt.Sprintf("acc-%d", userSeq), Currency: currency,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u.ID, a.ID
}

func TestTriggerRejectsUnbalancedAtCommit(t *testing.T) {
	ctx := context.Background()
	uid, accID := newUserWithAccount(t, "USD")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var txID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO transactions (user_id, currency, occurred_at) VALUES ($1,'USD',now()) RETURNING id`,
		uid).Scan(&txID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO postings (transaction_id, account_id, amount_minor) VALUES ($1,$2,999)`,
		txID, accID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("unbalanced transaction must fail at commit")
	}
}

func TestCreateIdempotentReplay(t *testing.T) {
	ctx := context.Background()
	uid, accID := newUserWithAccount(t, "USD")

	in := service.CreateInput{
		UserID: uid, AccountID: accID, AmountMinor: 1500, Currency: "USD",
		OccurredAt: time.Now().UTC(), Note: "replay test", IdempotencyKey: fmt.Sprintf("key-%d", uid),
	}
	first, err := txSvc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Postings) != 2 {
		t.Fatalf("want 2 postings, got %d", len(first.Postings))
	}

	second, err := txSvc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("replay created a new transaction: %d vs %d", second.ID, first.ID)
	}

	var nTx, nOutbox int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE user_id=$1`, uid).Scan(&nTx); err != nil {
		t.Fatal(err)
	}
	if nTx != 1 {
		t.Fatalf("want exactly 1 transaction, got %d", nTx)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE (payload->>'user_id')::bigint = $1`, uid).Scan(&nOutbox); err != nil {
		t.Fatal(err)
	}
	if nOutbox != 1 {
		t.Fatalf("want exactly 1 outbox event, got %d", nOutbox)
	}

	// Same key, different payload -> conflict.
	in.AmountMinor = 9999
	if _, err := txSvc.Create(ctx, in); !errors.Is(err, service.ErrIdempotencyConflict) {
		t.Fatalf("want ErrIdempotencyConflict, got %v", err)
	}
}

func TestConcurrentSameKeyCreatesOnce(t *testing.T) {
	ctx := context.Background()
	uid, accID := newUserWithAccount(t, "USD")

	in := service.CreateInput{
		UserID: uid, AccountID: accID, AmountMinor: 700, Currency: "USD",
		OccurredAt: time.Now().UTC(), IdempotencyKey: fmt.Sprintf("conc-%d", uid),
	}
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	ids := make([]int64, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			dto, err := txSvc.Create(ctx, in)
			errs[i], ids[i] = err, dto.ID
		}(i)
	}
	wg.Wait()

	var firstID int64
	for i := 0; i < n; i++ {
		switch {
		case errs[i] == nil:
			if firstID == 0 {
				firstID = ids[i]
			} else if ids[i] != firstID {
				t.Fatalf("two different transactions created: %d and %d", firstID, ids[i])
			}
		case errors.Is(errs[i], service.ErrIdempotencyInFlight):
			// acceptable: caller is told to retry
		default:
			t.Fatalf("unexpected error: %v", errs[i])
		}
	}

	var nTx int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE user_id=$1`, uid).Scan(&nTx); err != nil {
		t.Fatal(err)
	}
	if nTx != 1 {
		t.Fatalf("want exactly 1 transaction after %d concurrent requests, got %d", n, nTx)
	}
}

func TestTransferAndBalances(t *testing.T) {
	ctx := context.Background()
	uid, acc1 := newUserWithAccount(t, "USD")
	acc2, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "second", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: acc1, AmountMinor: 1000, Currency: "USD", OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := txSvc.Transfer(ctx, service.TransferInput{
		UserID: uid, FromAccountID: acc1, ToAccountID: acc2.ID, AmountMinor: 400,
		Currency: "USD", OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	b1, err := balSvc.CurrentBalance(ctx, acc1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b2, err := balSvc.CurrentBalance(ctx, acc2.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if b1.Balance != 600 || b2.Balance != 400 {
		t.Fatalf("balances wrong: acc1=%d (want 600) acc2=%d (want 400)", b1.Balance, b2.Balance)
	}
}

func TestOwnershipAndCurrencyChecks(t *testing.T) {
	ctx := context.Background()
	uid1, acc1 := newUserWithAccount(t, "USD")
	uid2, _ := newUserWithAccount(t, "USD")

	// Another user's account must be rejected.
	if _, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid2, AccountID: acc1, AmountMinor: 100, Currency: "USD", OccurredAt: time.Now().UTC(),
	}); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}

	// Currency mismatch must be rejected.
	if _, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid1, AccountID: acc1, AmountMinor: 100, Currency: "EUR", OccurredAt: time.Now().UTC(),
	}); !errors.Is(err, service.ErrCurrencyMismatch) {
		t.Fatalf("want ErrCurrencyMismatch, got %v", err)
	}
}

func TestReversal(t *testing.T) {
	ctx := context.Background()
	uid, accID := newUserWithAccount(t, "USD")

	orig, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accID, AmountMinor: 2500, Currency: "USD", OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	rev, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: orig.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rev.ReversalOf != orig.ID {
		t.Fatalf("reversal_of = %d, want %d", rev.ReversalOf, orig.ID)
	}

	b, err := balSvc.CurrentBalance(ctx, accID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if b.Balance != 0 {
		t.Fatalf("balance after reversal = %d, want 0", b.Balance)
	}

	// Reversing twice must fail via the DB unique constraint.
	if _, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: orig.ID}); !errors.Is(err, service.ErrAlreadyReversed) {
		t.Fatalf("want ErrAlreadyReversed, got %v", err)
	}
	// Reversing a reversal is rejected.
	if _, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: rev.ID}); !errors.Is(err, service.ErrIsReversal) {
		t.Fatalf("want ErrIsReversal, got %v", err)
	}
	// Another user cannot reverse my transaction.
	other, _ := newUserWithAccount(t, "USD")
	_ = other
	uidOther, _ := newUserWithAccount(t, "USD")
	if _, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uidOther, TransactionID: orig.ID}); !errors.Is(err, service.ErrTxNotFound) {
		t.Fatalf("want ErrTxNotFound for foreign user, got %v", err)
	}
}

func TestBackdatedPostingStillCounts(t *testing.T) {
	ctx := context.Background()
	uid, accID := newUserWithAccount(t, "USD")

	if _, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accID, AmountMinor: 1000, Currency: "USD", OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate the nightly snapshot worker for yesterday: postings created
	// up to yesterday's end-of-day (none — our posting was created today).
	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	day := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := day.Add(24*time.Hour - time.Nanosecond)
	sum, err := q.SumPostingsUpTo(ctx, sqlc.SumPostingsUpToParams{AccountID: accID, Cutoff: cutoff})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertBalanceSnapshot(ctx, sqlc.UpsertBalanceSnapshotParams{
		AccountID: accID, AsOfDate: day, BalanceMinor: sum,
	}); err != nil {
		t.Fatal(err)
	}

	// A transaction backdated 30 days lands AFTER the snapshot cutoff by
	// created_at, so it must still appear in the current balance.
	if _, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accID, AmountMinor: 500, Currency: "USD",
		OccurredAt: time.Now().UTC().Add(-30 * 24 * time.Hour), Note: "backdated",
	}); err != nil {
		t.Fatal(err)
	}

	b, err := balSvc.CurrentBalance(ctx, accID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if b.Balance != 1500 {
		t.Fatalf("balance = %d, want 1500 (backdated posting lost?)", b.Balance)
	}
}

func TestOutboxPublishesAndMarksProcessed(t *testing.T) {
	ctx := context.Background()
	uid, accID := newUserWithAccount(t, "USD")

	sub := rdb.Subscribe(ctx, "events:TransactionCreated")
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil { // wait for subscription
		t.Fatal(err)
	}

	if _, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accID, AmountMinor: 100, Currency: "USD", OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	w := worker.NewOutboxWorker(pool, rdb, zap.NewNop())
	w.ProcessBatch(ctx)

	recvCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	msg, err := sub.ReceiveMessage(recvCtx)
	if err != nil {
		t.Fatalf("no event published: %v", err)
	}
	if !strings.Contains(msg.Payload, `"postings"`) {
		t.Fatalf("unexpected payload: %s", msg.Payload)
	}

	var unprocessed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE processed_at IS NULL`).Scan(&unprocessed); err != nil {
		t.Fatal(err)
	}
	if unprocessed != 0 {
		t.Fatalf("%d outbox events left unprocessed", unprocessed)
	}
}

func TestMonthlySummaryAndCacheInvalidation(t *testing.T) {
	ctx := context.Background()
	uid, accID := newUserWithAccount(t, "USD")
	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	if _, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accID, AmountMinor: 1000, Currency: "USD", OccurredAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	s1, err := sumSvc.GetMonthlySummary(ctx, accID, month)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Inflow != 1000 || s1.Net != 1000 {
		t.Fatalf("summary wrong: %+v", s1)
	}

	// Second tx must invalidate the cache; the summary must move.
	if _, err := txSvc.Create(ctx, service.CreateInput{
		UserID: uid, AccountID: accID, AmountMinor: -250, Currency: "USD", OccurredAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	s2, err := sumSvc.GetMonthlySummary(ctx, accID, month)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Outflow != 250 || s2.Net != 750 {
		t.Fatalf("stale summary after invalidation: %+v", s2)
	}
}
