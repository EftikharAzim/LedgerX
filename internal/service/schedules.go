package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	"github.com/EftikharAzim/ledgerx/internal/planning"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrScheduleInput identifies an invalid recurring commitment.
var ErrScheduleInput = errors.New("check the wallet, amount, category and recurrence dates; a new schedule can start within the past year or in the future")

// ErrScheduleNotFound hides missing or foreign schedules.
var ErrScheduleNotFound = errors.New("recurring schedule not found")

// ErrScheduleChanged protects edits and status controls from stale screens.
var ErrScheduleChanged = errors.New("this schedule changed; reload it before continuing")

// ErrScheduleEnded prevents changing a terminal schedule.
var ErrScheduleEnded = errors.New("this schedule has ended; create a new schedule to restart it")

// ErrScheduleLimit bounds a user's recurring planning workload.
var ErrScheduleLimit = errors.New("recurring planning limit reached: 500 active/paused schedules, 10,000 saved schedules or 50,000 plans")

// ScheduleInput defines future commitments without creating ledger postings.
type ScheduleInput struct {
	AccountID    int64  `json:"account_id"`
	Currency     string `json:"currency"`
	AmountMinor  int64  `json:"amount_minor"`
	CategoryID   int64  `json:"category_id"`
	Note         string `json:"note"`
	Cadence      string `json:"cadence"`
	FirstDueDate string `json:"first_due_date"`
	EndDate      string `json:"end_date"`
	Revision     int64  `json:"revision"`
}

// ScheduleView adds display labels to portable recurrence metadata.
type ScheduleView struct {
	planning.Schedule
	AccountName  string `json:"account_name"`
	CategoryName string `json:"category_name"`
	NextDueDate  string `json:"next_due_date"`
}

// ScheduleService materializes pending plans under the user's ledger lock.
type ScheduleService struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
	now  func() time.Time
}

// NewScheduleService shares existing ownership and transaction boundaries.
func NewScheduleService(pool *pgxpool.Pool, q *sqlc.Queries, clocks ...func() time.Time) *ScheduleService {
	clock := time.Now
	if len(clocks) > 0 && clocks[0] != nil {
		clock = clocks[0]
	}
	return &ScheduleService{pool: pool, q: q, now: clock}
}

const publicSchedule = `to_jsonb(s)-'user_id'-'creation_key'-'creation_hash'-'last_edit_key'-'last_edit_hash'`

func readSchedule(ctx context.Context, tx pgx.Tx, uid, id int64) (planning.Schedule, error) {
	var out planning.Schedule
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT `+publicSchedule+` FROM recurring_schedules s WHERE user_id=$1 AND id=$2`, uid, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrScheduleNotFound
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}

func scheduleInput(in ScheduleInput) (ScheduleInput, time.Time, error) {
	in.Note = strings.TrimSpace(in.Note)
	first, err := planning.Date(in.FirstDueDate)
	if err != nil || in.AccountID <= 0 || in.CategoryID < 0 || in.AmountMinor == 0 || !safeMinor(in.AmountMinor) || len([]rune(in.Note)) > 1000 || strings.ContainsRune(in.Note, 0) || (in.Cadence != "weekly" && in.Cadence != "monthly" && in.Cadence != "yearly") {
		return in, first, ErrScheduleInput
	}
	if in.EndDate != "" {
		end, err := planning.Date(in.EndDate)
		if err != nil || end.Before(first) {
			return in, first, ErrScheduleInput
		}
	}
	return in, first, nil
}

// Save creates once for a request key, or applies a reviewed revision to future plans.
func (s *ScheduleService) Save(ctx context.Context, uid, id int64, key string, in ScheduleInput) (planning.Schedule, error) {
	var empty planning.Schedule
	in, first, err := scheduleInput(in)
	if err != nil || key == "" || len(key) > 128 {
		return empty, ErrScheduleInput
	}
	hash := importing.Hash(in)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return empty, err
	}
	today := planning.Today(s.now())
	var current planning.Schedule
	if id == 0 {
		var prior int64
		var previousHash string
		err := tx.QueryRow(ctx, "SELECT id,creation_hash FROM recurring_schedules WHERE user_id=$1 AND creation_key=$2", uid, key).Scan(&prior, &previousHash)
		if err == nil {
			if previousHash != hash {
				return empty, ErrIdempotencyConflict
			}
			return readSchedule(ctx, tx, uid, prior)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return empty, err
		}
		if first.Before(today.AddDate(-1, 0, 0)) {
			return empty, ErrScheduleInput
		}
		var total, open int
		if err := tx.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE status<>'ended') FROM recurring_schedules WHERE user_id=$1", uid).Scan(&total, &open); err != nil {
			return empty, err
		}
		if total >= 10000 || open >= 500 {
			return empty, ErrScheduleLimit
		}
	} else {
		current, err = readSchedule(ctx, tx, uid, id)
		if err != nil {
			return empty, err
		}
		var lastKey, lastHash pgtype.Text
		if err := tx.QueryRow(ctx, "SELECT last_edit_key,last_edit_hash FROM recurring_schedules WHERE user_id=$1 AND id=$2", uid, id).Scan(&lastKey, &lastHash); err != nil {
			return empty, err
		}
		if lastKey.String == key {
			if lastHash.String != hash {
				return empty, ErrIdempotencyConflict
			}
			return current, nil
		}
		if current.Status == "ended" {
			return empty, ErrScheduleEnded
		}
		if current.Revision != in.Revision {
			return empty, ErrScheduleChanged
		}
		if (current.Cadence != in.Cadence || current.FirstDueDate != in.FirstDueDate) && first.Before(today) {
			return empty, ErrScheduleInput
		}
	}
	a, err := loadLedgerContext(ctx, q, uid, in.AccountID)
	if err != nil {
		return empty, err
	}
	if !a.Active {
		return empty, ErrAccountInactive
	}
	if a.Currency != in.Currency {
		return empty, ErrCurrencyMismatch
	}
	if in.CategoryID != 0 && (id == 0 || in.CategoryID != current.CategoryID || (in.AmountMinor > 0) != (current.AmountMinor > 0)) {
		if err := validateEntryCategory(ctx, q, uid, in.CategoryID, in.AmountMinor, "standard"); err != nil {
			return empty, err
		}
	}
	end := pgtype.Date{}
	if in.EndDate != "" {
		end.Time, _ = planning.Date(in.EndDate)
		end.Valid = true
	}
	if id == 0 {
		err = tx.QueryRow(ctx, `INSERT INTO recurring_schedules(user_id,account_id,currency,amount_minor,category_id,note,cadence,first_due_date,end_date,generate_from,creation_key,creation_hash)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$8,$10,$11) RETURNING id`, uid, in.AccountID, in.Currency, in.AmountMinor, pgtype.Int8{Int64: in.CategoryID, Valid: in.CategoryID != 0}, in.Note, in.Cadence, first, end, key, hash).Scan(&id)
	} else {
		_, err = tx.Exec(ctx, "UPDATE planned_transactions SET status='canceled',cancellation_reason='rescheduled' WHERE user_id=$1 AND schedule_id=$2 AND status='pending' AND scheduled_date>=$3", uid, id, today)
		if err != nil {
			return empty, err
		}
		_, err = tx.Exec(ctx, `UPDATE recurring_schedules SET account_id=$3,currency=$4,amount_minor=$5,category_id=$6,note=$7,cadence=$8,first_due_date=$9,end_date=$10,generate_from=$11,
 generated_through=NULL,refreshed_on=NULL,revision=revision+1,last_edit_key=$12,last_edit_hash=$13,updated_at=now() WHERE user_id=$1 AND id=$2`, uid, id, in.AccountID, in.Currency, in.AmountMinor, pgtype.Int8{Int64: in.CategoryID, Valid: in.CategoryID != 0}, in.Note, in.Cadence, first, end, today, key, hash)
	}
	if err != nil {
		return empty, err
	}
	current, err = readSchedule(ctx, tx, uid, id)
	if err != nil {
		return empty, err
	}
	if current.Status == "active" {
		if err := generateSchedule(ctx, tx, uid, current, today); err != nil {
			return empty, err
		}
	}
	current, err = readSchedule(ctx, tx, uid, id)
	if err != nil {
		return empty, err
	}
	return current, tx.Commit(ctx)
}

func generateSchedule(ctx context.Context, tx pgx.Tx, uid int64, sc planning.Schedule, today time.Time) error {
	first, err := planning.Date(sc.FirstDueDate)
	if err != nil {
		return ErrScheduleInput
	}
	from, err := planning.Date(sc.GenerateFrom)
	if err != nil {
		return ErrScheduleInput
	}
	if sc.GeneratedThrough != "" {
		through, _ := planning.Date(sc.GeneratedThrough)
		if next := through.AddDate(0, 0, 1); next.After(from) {
			from = next
		}
	}
	horizon := today.AddDate(0, 0, 90)
	if horizon.Year() > 9999 {
		horizon = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	}
	until := horizon
	if sc.EndDate != "" {
		end, _ := planning.Date(sc.EndDate)
		if end.Before(until) {
			until = end
		}
	}
	dates, err := planning.Dates(first, sc.Cadence, from, until)
	if err != nil {
		return ErrScheduleInput
	}
	// Show a first due date beyond the preview window without generating years of plans.
	if sc.GeneratedThrough == "" && first.After(horizon) && !first.Before(from) {
		dates = append(dates, first)
	}
	for _, at := range dates {
		_, err := tx.Exec(ctx, `INSERT INTO planned_transactions(user_id,account_id,currency,amount_minor,category_id,occurred_at,note,entry_type,date_only,schedule_id,scheduled_date,schedule_revision)
 VALUES($1,$2,$3,$4,$5,$6::timestamptz,$7,'standard',true,$8,($6::timestamptz AT TIME ZONE 'UTC')::date,$9)
 ON CONFLICT(schedule_id,scheduled_date) DO UPDATE SET account_id=excluded.account_id,currency=excluded.currency,amount_minor=excluded.amount_minor,category_id=excluded.category_id,note=excluded.note,
 status='pending',cancellation_reason='',schedule_revision=excluded.schedule_revision
 WHERE planned_transactions.status='canceled' AND planned_transactions.cancellation_reason IN ('paused','rescheduled')`, uid, sc.AccountID, sc.Currency, sc.AmountMinor, pgtype.Int8{Int64: sc.CategoryID, Valid: sc.CategoryID != 0}, at, sc.Note, sc.ID, sc.Revision)
		if err != nil {
			return err
		}
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM planned_transactions WHERE user_id=$1", uid).Scan(&count); err != nil {
		return err
	}
	if count > 50000 {
		return ErrScheduleLimit
	}
	_, err = tx.Exec(ctx, "UPDATE recurring_schedules SET generated_through=$3,refreshed_on=$4 WHERE user_id=$1 AND id=$2", uid, sc.ID, horizon, today)
	return err
}

// Action pauses future occurrences, resumes from today, or ends a series.
// Already-due plans remain available for explicit payment/skip confirmation.
func (s *ScheduleService) Action(ctx context.Context, uid, id, revision int64, action string) (planning.Schedule, error) {
	var empty planning.Schedule
	target := map[string]string{"pause": "paused", "resume": "active", "end": "ended"}[action]
	if target == "" {
		return empty, ErrScheduleInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return empty, err
	}
	sc, err := readSchedule(ctx, tx, uid, id)
	if err != nil {
		return empty, err
	}
	if sc.Status == target {
		return sc, nil
	}
	if sc.Status == "ended" {
		return empty, ErrScheduleEnded
	}
	if sc.Revision != revision {
		return empty, ErrScheduleChanged
	}
	today := planning.Today(s.now())
	if target != "active" {
		_, err = tx.Exec(ctx, "UPDATE planned_transactions SET status='canceled',cancellation_reason=$4 WHERE user_id=$1 AND schedule_id=$2 AND status='pending' AND scheduled_date>$3", uid, id, today, target)
		if err != nil {
			return empty, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE recurring_schedules SET status=$3,revision=revision+1,generate_from=CASE WHEN $3='active' THEN $4 ELSE generate_from END,
 generated_through=CASE WHEN $3='active' THEN NULL ELSE generated_through END,refreshed_on=NULL,updated_at=now() WHERE user_id=$1 AND id=$2`, uid, id, target, today)
	if err != nil {
		return empty, err
	}
	sc, err = readSchedule(ctx, tx, uid, id)
	if err != nil {
		return empty, err
	}
	if target == "active" {
		if err := generateSchedule(ctx, tx, uid, sc, today); err != nil {
			return empty, err
		}
	}
	sc, err = readSchedule(ctx, tx, uid, id)
	if err != nil {
		return empty, err
	}
	return sc, tx.Commit(ctx)
}

// Sync materializes a 90-day window once per UTC day, without posting money.
func (s *ScheduleService) Sync(ctx context.Context, uid int64, now time.Time) error {
	today := planning.Today(now)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.q.WithTx(tx).LockUserLedger(ctx, uid); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT `+publicSchedule+` FROM recurring_schedules s WHERE user_id=$1 AND status='active' AND (refreshed_on IS NULL OR refreshed_on<$2) ORDER BY id`, uid, today)
	if err != nil {
		return err
	}
	schedules := []planning.Schedule{}
	for rows.Next() {
		var raw []byte
		var sc planning.Schedule
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal(raw, &sc); err != nil {
			rows.Close()
			return err
		}
		schedules = append(schedules, sc)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, sc := range schedules {
		if err := generateSchedule(ctx, tx, uid, sc, today); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// SyncDue is safe with several app instances; user locks and unique dates dedupe work.
func (s *ScheduleService) SyncDue(ctx context.Context, now time.Time) error {
	rows, err := s.pool.Query(ctx, "SELECT DISTINCT user_id FROM recurring_schedules WHERE status='active' AND (refreshed_on IS NULL OR refreshed_on<$1) ORDER BY user_id LIMIT 100", planning.Today(now))
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return err
	}
	var failures []error
	for _, uid := range ids {
		if err := s.Sync(ctx, uid, now); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// List returns open schedules and the most recent ended schedules, up to 500.
func (s *ScheduleService) List(ctx context.Context, uid int64) ([]ScheduleView, error) {
	rows, err := s.pool.Query(ctx, `SELECT (`+publicSchedule+`) || jsonb_build_object('account_name',a.name,'category_name',COALESCE(c.name,'Uncategorized'),'next_due_date',COALESCE((SELECT min(p.scheduled_date)::text FROM planned_transactions p WHERE p.schedule_id=s.id AND p.status='pending'),''))
 FROM recurring_schedules s JOIN accounts a ON a.id=s.account_id LEFT JOIN categories c ON c.id=s.category_id WHERE s.user_id=$1 ORDER BY CASE s.status WHEN 'active' THEN 0 WHEN 'paused' THEN 1 ELSE 2 END,s.id DESC LIMIT 500`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScheduleView{}
	for rows.Next() {
		var raw []byte
		var sc ScheduleView
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &sc); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}
