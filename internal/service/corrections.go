package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Correction errors explain review/recovery without exposing other workspaces.
var (
	ErrCorrectionInput       = errors.New("check the amount, date, accounts, category, and transaction type")
	ErrCorrectionChanged     = errors.New("this record or an affected account changed; reload and review the correction again")
	ErrCorrectionUnchanged   = errors.New("change at least one field before reviewing a correction")
	ErrCorrectionUnsupported = errors.New("this legacy transaction needs its dedicated reversal action")
	ErrCorrectionStart       = errors.New("keep one starting balance per account and place it no later than the earliest active transaction")
)

// CorrectionDraft describes the replacement a customer is reviewing.
type CorrectionDraft struct {
	AccountID     int64     `json:"account_id"`
	ToAccountID   int64     `json:"to_account_id,omitempty"`
	AmountMinor   int64     `json:"amount_minor"`
	Currency      string    `json:"currency"`
	EntryType     string    `json:"entry_type"`
	ExcludeReport bool      `json:"exclude_from_reports,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
	DateOnly      bool      `json:"date_only"`
	Note          string    `json:"note"`
	CategoryID    int64     `json:"category_id"`
}

// CorrectionContext supplies the current draft and its optimistic review token.
type CorrectionContext struct {
	TransactionID int64           `json:"transaction_id"`
	Draft         CorrectionDraft `json:"draft"`
	SourceToken   string          `json:"source_token"`
}

// CorrectionInput is previewed before a retry-safe correction is committed.
type CorrectionInput struct {
	Draft        CorrectionDraft `json:"draft"`
	SourceToken  string          `json:"source_token"`
	PreviewToken string          `json:"preview_token,omitempty"`
}

// AccountBalanceChange makes the effect of replacing a record explicit.
type AccountBalanceChange struct {
	AccountID   int64  `json:"account_id"`
	Name        string `json:"name"`
	Currency    string `json:"currency"`
	BeforeMinor int64  `json:"before_minor"`
	ChangeMinor int64  `json:"change_minor"`
	AfterMinor  int64  `json:"after_minor"`
}

// CorrectionPlan is a server-calculated before/after review without ledger writes.
type CorrectionPlan struct {
	OriginalID   int64                  `json:"original_id"`
	Old          CorrectionDraft        `json:"old"`
	New          CorrectionDraft        `json:"new"`
	Balances     []AccountBalanceChange `json:"balances"`
	SourceToken  string                 `json:"source_token"`
	PreviewToken string                 `json:"preview_token"`
}

// CorrectionReceipt links the original, balancing reversal, and replacement.
type CorrectionReceipt struct {
	OriginalID    int64                  `json:"original_id"`
	ReversalID    int64                  `json:"reversal_id"`
	ReplacementID int64                  `json:"replacement_id,omitempty"`
	Old           CorrectionDraft        `json:"old"`
	New           CorrectionDraft        `json:"new"`
	Balances      []AccountBalanceChange `json:"balances"`
	CompletedAt   time.Time              `json:"completed_at"`
}

// CorrectionService atomically reverses and replaces records while retaining history.
type CorrectionService struct {
	pool    *pgxpool.Pool
	q       *sqlc.Queries
	summary *SummaryService
}

// NewCorrectionService shares the same ledger writer/lock as imports and entry.
func NewCorrectionService(pool *pgxpool.Pool, q *sqlc.Queries, summary *SummaryService) *CorrectionService {
	return &CorrectionService{pool, q, summary}
}

type correctionSource struct {
	context      CorrectionContext
	legs         []Leg
	importSource int64
}

func readCorrectionSource(ctx context.Context, q *sqlc.Queries, uid, id int64) (correctionSource, error) {
	var out correctionSource
	row, err := q.GetTransactionForUserForUpdate(ctx, sqlc.GetTransactionForUserForUpdateParams{ID: id, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrTxNotFound
	}
	if err != nil {
		return out, err
	}
	if row.ReversalOf.Valid {
		return out, ErrIsReversal
	}
	origin, err := q.GetCorrectionOrigin(ctx, sqlc.GetCorrectionOriginParams{ID: id, UserID: uid})
	if err != nil {
		return out, err
	}
	if origin.ReversedBy != 0 {
		return out, ErrAlreadyReversed
	}
	posts, err := q.ListPostingsByTransaction(ctx, id)
	if err != nil {
		return out, err
	}
	if len(posts) != 2 || posts[0].AmountMinor+posts[1].AmountMinor != 0 {
		return out, ErrCorrectionUnsupported
	}
	d := CorrectionDraft{ExcludeReport: row.ExcludeFromReports, Currency: row.Currency, EntryType: row.EntryType, OccurredAt: row.OccurredAt.UTC(), Note: row.Note.String, CategoryID: row.CategoryID.Int64, DateOnly: origin.DateOnly}
	normals := 0
	for _, p := range posts {
		a, err := q.GetAccount(ctx, p.AccountID)
		if err != nil {
			return out, err
		}
		if a.UserID != uid {
			return out, ErrForbidden
		}
		out.legs = append(out.legs, Leg{AccountID: p.AccountID, AmountMinor: p.AmountMinor})
		if a.Kind == "normal" {
			normals++
			if p.AmountMinor < 0 || d.AccountID == 0 {
				d.AccountID = a.ID
				d.AmountMinor = p.AmountMinor
			}
		}
	}
	if normals == 2 {
		if row.EntryType != "standard" {
			return out, ErrCorrectionUnsupported
		}
		d.EntryType = "transfer"
		d.CategoryID = 0
		for _, p := range posts {
			if p.AmountMinor > 0 {
				d.ToAccountID = p.AccountID
			}
		}
	} else if normals != 1 {
		return out, ErrCorrectionUnsupported
	}
	out.context = CorrectionContext{TransactionID: id, Draft: d}
	out.context.SourceToken = importing.Hash(out.context)
	out.importSource = origin.ImportSourceID
	return out, nil
}

func (s *CorrectionService) begin(ctx context.Context, uid int64) (pgx.Tx, *sqlc.Queries, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}
	return tx, q, nil
}

// Context loads an owned, active original for the edit form.
func (s *CorrectionService) Context(ctx context.Context, uid, id int64) (CorrectionContext, error) {
	tx, q, err := s.begin(ctx, uid)
	if err != nil {
		return CorrectionContext{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	source, err := readCorrectionSource(ctx, q, uid, id)
	if err != nil {
		return CorrectionContext{}, err
	}
	return source.context, tx.Commit(ctx)
}

func correctionPlan(ctx context.Context, q *sqlc.Queries, uid, id int64, in CorrectionInput) (CorrectionPlan, correctionSource, []Leg, error) {
	source, err := readCorrectionSource(ctx, q, uid, id)
	if err != nil {
		return CorrectionPlan{}, source, nil, err
	}
	fail := func(err error) (CorrectionPlan, correctionSource, []Leg, error) {
		return CorrectionPlan{}, source, nil, err
	}
	if in.SourceToken == "" || in.SourceToken != source.context.SourceToken {
		return fail(ErrCorrectionChanged)
	}
	d := in.Draft
	d.OccurredAt = d.OccurredAt.UTC()
	d.Note = strings.TrimSpace(d.Note)
	old := source.context.Draft
	d.ExcludeReport = old.ExcludeReport
	if d.Currency != old.Currency {
		return fail(ErrCurrencyMismatch)
	}
	if !safeMinor(d.AmountMinor) || d.CategoryID < 0 || len([]rune(d.Note)) > 1000 || d.OccurredAt.IsZero() || d.OccurredAt.Year() < 1 || d.OccurredAt.Year() > 9999 || d.OccurredAt.After(time.Now().UTC().Add(time.Minute)) {
		return fail(ErrCorrectionInput)
	}
	if d.DateOnly && (d.OccurredAt.Hour() != 0 || d.OccurredAt.Minute() != 0 || d.OccurredAt.Second() != 0 || d.OccurredAt.Nanosecond() != 0) {
		return fail(ErrCorrectionInput)
	}
	if old.EntryType == "transfer" {
		if d.EntryType != "transfer" || d.AmountMinor >= 0 || d.ToAccountID == 0 || d.AccountID == d.ToAccountID || d.CategoryID != 0 {
			return fail(ErrCorrectionInput)
		}
	} else if d.ToAccountID != 0 {
		return fail(ErrCorrectionInput)
	}
	if old.EntryType == "opening_balance" {
		if d.EntryType != "opening_balance" || d.CategoryID != 0 {
			return fail(ErrCorrectionInput)
		}
	} else if old.EntryType != "transfer" {
		if (d.EntryType != "standard" && d.EntryType != "refund") || d.AmountMinor == 0 || (d.EntryType == "refund" && d.AmountMinor <= 0) {
			return fail(ErrCorrectionInput)
		}
	}
	if importing.Hash(d) == importing.Hash(old) {
		return fail(ErrCorrectionUnchanged)
	}
	ids := map[int64]bool{d.AccountID: true}
	if d.ToAccountID != 0 {
		ids[d.ToAccountID] = true
	}
	contexts := map[int64]AccountLedgerContext{}
	for aid := range ids {
		a, err := loadLedgerContext(ctx, q, uid, aid)
		if err != nil {
			return fail(err)
		}
		if !a.Active {
			return fail(ErrAccountInactive)
		}
		if a.Currency != d.Currency {
			return fail(ErrCurrencyMismatch)
		}
		contexts[aid] = a
		if old.EntryType != "opening_balance" && a.TrackingStartAt != nil && d.OccurredAt.Before(*a.TrackingStartAt) {
			return fail(ErrCorrectionStart)
		}
	}
	if d.CategoryID != 0 && d.CategoryID != old.CategoryID {
		if err := validateEntryCategory(ctx, q, uid, d.CategoryID, d.AmountMinor, d.EntryType); err != nil {
			return fail(ErrInvalidCategory)
		}
	}
	if old.EntryType == "opening_balance" {
		original, err := loadLedgerContext(ctx, q, uid, old.AccountID)
		if err != nil {
			return fail(err)
		}
		target := contexts[d.AccountID]
		if original.OpeningCount != 1 || original.OpeningID != id || (d.AccountID != old.AccountID && target.OpeningCount != 0) {
			return fail(ErrCorrectionStart)
		}
		earliest, err := q.EarliestActiveAccountActivity(ctx, d.AccountID)
		if err != nil {
			return fail(err)
		}
		if d.OccurredAt.After(earliest) {
			return fail(ErrCorrectionStart)
		}
	}
	delta := map[int64]int64{}
	for _, leg := range source.legs {
		a, err := q.GetAccount(ctx, leg.AccountID)
		if err != nil {
			return fail(err)
		}
		if a.Kind == "normal" {
			delta[a.ID] -= leg.AmountMinor
			if _, ok := contexts[a.ID]; !ok {
				c, err := loadLedgerContext(ctx, q, uid, a.ID)
				if err != nil {
					return fail(err)
				}
				contexts[a.ID] = c
			}
		}
	}
	delta[d.AccountID] += d.AmountMinor
	legs := []Leg{}
	if d.AmountMinor != 0 {
		legs = append(legs, Leg{AccountID: d.AccountID, AmountMinor: d.AmountMinor})
		if d.EntryType == "transfer" {
			legs = append(legs, Leg{AccountID: d.ToAccountID, AmountMinor: -d.AmountMinor})
			delta[d.ToAccountID] -= d.AmountMinor
		}
	}
	sorted := make([]int64, 0, len(contexts))
	for aid := range contexts {
		sorted = append(sorted, aid)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	plan := CorrectionPlan{OriginalID: id, Old: old, New: d, Balances: []AccountBalanceChange{}, SourceToken: source.context.SourceToken}
	tokens := []string{}
	for _, aid := range sorted {
		a := contexts[aid]
		after, err := addMinor(a.BalanceMinor, delta[aid])
		if err != nil {
			return fail(err)
		}
		plan.Balances = append(plan.Balances, AccountBalanceChange{AccountID: aid, Name: a.Name, Currency: a.Currency, BeforeMinor: a.BalanceMinor, ChangeMinor: delta[aid], AfterMinor: after})
		tokens = append(tokens, a.Token)
	}
	plan.PreviewToken = importing.Hash(struct {
		Plan     CorrectionPlan
		Accounts []string
	}{plan, tokens})
	return plan, source, legs, nil
}

// Preview calculates every affected balance without posting a correction.
func (s *CorrectionService) Preview(ctx context.Context, uid, id int64, in CorrectionInput) (CorrectionPlan, error) {
	tx, q, err := s.begin(ctx, uid)
	if err != nil {
		return CorrectionPlan{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	plan, _, _, err := correctionPlan(ctx, q, uid, id, in)
	if err != nil {
		return plan, err
	}
	return plan, tx.Commit(ctx)
}

// Correct commits a reversal, replacement, origin links, and receipt atomically.
func (s *CorrectionService) Correct(ctx context.Context, uid, id int64, key string, in CorrectionInput) (CorrectionReceipt, error) {
	var empty CorrectionReceipt
	if key == "" || len(key) > 128 {
		return empty, ErrCorrectionInput
	}
	in.Draft.OccurredAt = in.Draft.OccurredAt.UTC()
	in.Draft.Note = strings.TrimSpace(in.Draft.Note)
	hash := importing.Hash(struct {
		ID    int64
		Input CorrectionInput
	}{id, in})
	tx, q, err := s.begin(ctx, uid)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	previous, err := q.GetCorrectionByKey(ctx, sqlc.GetCorrectionByKeyParams{UserID: uid, RequestKey: key})
	if err == nil {
		if previous.RequestHash != hash {
			return empty, ErrIdempotencyConflict
		}
		err = json.Unmarshal(previous.Receipt, &empty)
		return empty, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	plan, source, legs, err := correctionPlan(ctx, q, uid, id, in)
	if err != nil {
		return empty, err
	}
	if in.PreviewToken == "" || in.PreviewToken != plan.PreviewToken {
		return empty, ErrCorrectionChanged
	}
	reversal, _, _, err := appendReversal(ctx, q, uid, id)
	if err != nil {
		return empty, err
	}
	receipt := CorrectionReceipt{OriginalID: id, ReversalID: reversal.ID, Old: plan.Old, New: plan.New, Balances: plan.Balances, CompletedAt: time.Now().UTC()}
	if len(legs) > 0 {
		entryType := plan.New.EntryType
		if entryType != "transfer" {
			external, err := q.EnsureExternalAccount(ctx, sqlc.EnsureExternalAccountParams{UserID: uid, Currency: plan.New.Currency})
			if err != nil {
				return empty, err
			}
			legs = append(legs, Leg{AccountID: external, AmountMinor: -plan.New.AmountMinor})
		} else {
			entryType = "standard"
		}
		replacement, err := appendLedger(ctx, q, ledgerWrite{UserID: uid, Currency: plan.New.Currency, At: plan.New.OccurredAt, Note: plan.New.Note, ExcludeReport: plan.New.ExcludeReport, CategoryID: plan.New.CategoryID, EntryType: entryType, Legs: legs})
		if err != nil {
			return empty, err
		}
		receipt.ReplacementID = replacement.ID
	}
	if plan.New.EntryType == "opening_balance" {
		if err := q.SetTrackingStart(ctx, sqlc.SetTrackingStartParams{AccountID: plan.New.AccountID, TrackingStartAt: pgtype.Timestamptz{Time: plan.New.OccurredAt, Valid: true}}); err != nil {
			return empty, err
		}
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return empty, err
	}
	if err := q.CreateCorrection(ctx, sqlc.CreateCorrectionParams{UserID: uid, OriginalID: id, ReversalID: reversal.ID, ReplacementID: pgtype.Int8{Int64: receipt.ReplacementID, Valid: receipt.ReplacementID != 0}, ImportSourceID: pgtype.Int8{Int64: source.importSource, Valid: source.importSource != 0}, DateOnly: plan.New.DateOnly, RequestKey: key, RequestHash: hash, Receipt: data}); err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	if s.summary != nil {
		affected := map[int64]bool{}
		for _, leg := range source.legs {
			affected[leg.AccountID] = true
		}
		for _, leg := range legs {
			affected[leg.AccountID] = true
		}
		for aid := range affected {
			for _, at := range []time.Time{plan.Old.OccurredAt, plan.New.OccurredAt, reversal.OccurredAt} {
				s.summary.Invalidate(ctx, aid, at)
			}
		}
	}
	return receipt, nil
}

// Result recovers only the caller's exact write attempt after a lost response.
func (s *CorrectionService) Result(ctx context.Context, uid, id int64, key string) (CorrectionReceipt, error) {
	var receipt CorrectionReceipt
	if key == "" || len(key) > 128 {
		return receipt, ErrCorrectionInput
	}
	row, err := s.q.GetCorrectionByKey(ctx, sqlc.GetCorrectionByKeyParams{UserID: uid, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.OriginalID != id) {
		return receipt, ErrTxNotFound
	}
	if err != nil {
		return receipt, err
	}
	err = json.Unmarshal(row.Receipt, &receipt)
	return receipt, err
}
