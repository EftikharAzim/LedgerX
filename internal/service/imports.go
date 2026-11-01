package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	"github.com/EftikharAzim/ledgerx/internal/money"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Import errors are safe to explain without exposing another user's records.
var (
	ErrImportNotFound  = errors.New("import not found")
	ErrImportExpired   = errors.New("this preview expired; upload the file and preview it again")
	ErrImportChanged   = errors.New("account activity changed since this preview; refresh the preview before importing")
	ErrImportSelection = errors.New("choose valid preview rows and explicitly approve any selected duplicates")
	ErrImportCommitted = errors.New("this import is already completed with a different selection")
	ErrMoneyRange      = errors.New("the amount or combined balance exceeds the supported exact range")
)

// AccountLedgerContext is the current account state reviewed before importing.
type AccountLedgerContext struct {
	AccountID       int64      `json:"account_id"`
	Name            string     `json:"name"`
	Currency        string     `json:"currency"`
	Active          bool       `json:"active"`
	BalanceMinor    int64      `json:"balance_minor"`
	Revision        int64      `json:"revision"`
	EntryCount      int64      `json:"entry_count"`
	OpeningID       int64      `json:"opening_id"`
	OpeningMinor    int64      `json:"opening_minor"`
	OpeningAt       *time.Time `json:"opening_at,omitempty"`
	OpeningCount    int64      `json:"opening_count"`
	TrackingStartAt *time.Time `json:"tracking_start_at,omitempty"`
	Token           string     `json:"token"`
}

func loadLedgerContext(ctx context.Context, q *sqlc.Queries, uid, accountID int64) (AccountLedgerContext, error) {
	row, err := q.GetAccountLedgerContext(ctx, sqlc.GetAccountLedgerContextParams{AccountID: accountID, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountLedgerContext{}, ErrAccountNotFound
	}
	if err != nil {
		return AccountLedgerContext{}, err
	}
	if !safeMinor(row.BalanceMinor) {
		return AccountLedgerContext{}, ErrMoneyRange
	}
	out := AccountLedgerContext{AccountID: row.ID, Name: row.Name, Currency: row.Currency, Active: row.IsActive, BalanceMinor: row.BalanceMinor, Revision: row.Revision, EntryCount: row.EntryCount, OpeningID: row.OpeningID, OpeningMinor: row.OpeningMinor, OpeningCount: row.OpeningCount}
	if row.OpeningID != 0 {
		at := row.OpeningAt.UTC()
		out.OpeningAt = &at
	}
	if row.TrackingStartAt.Valid {
		at := row.TrackingStartAt.Time.UTC()
		out.TrackingStartAt = &at
	}
	out.Token = importing.Hash(out)
	return out, nil
}

func safeMinor(value int64) bool { return value >= -money.MaxSafeMinor && value <= money.MaxSafeMinor }
func addMinor(a, b int64) (int64, error) {
	if !safeMinor(a) || !safeMinor(b) || !safeMinor(a+b) {
		return 0, ErrMoneyRange
	}
	return a + b, nil
}

// StartingBalancePlan explicitly retains or corrects the opening component.
type StartingBalancePlan struct {
	Mode        string `json:"mode"`
	AmountMinor int64  `json:"amount_minor"`
	Date        string `json:"date"`
}

// ImportInput contains the file and mapping; normalized rows are server-owned.
type ImportInput struct {
	AccountID   int64               `json:"account_id"`
	Filename    string              `json:"filename"`
	CSV         string              `json:"csv"`
	SourceGroup string              `json:"source_group,omitempty"`
	Delimiter   string              `json:"delimiter"`
	Mapping     importing.Mapping   `json:"mapping"`
	Starting    StartingBalancePlan `json:"starting_balance"`
}

// ImportPreview is an immutable staged plan. Committing requires its row numbers.
type ImportPreview struct {
	Context          AccountLedgerContext `json:"context"`
	Rows             []importing.Row      `json:"rows"`
	Starting         StartingBalancePlan  `json:"starting_balance"`
	Mapping          importing.Mapping    `json:"mapping"`
	NewCategories    []string             `json:"new_categories"`
	PlanErrors       []string             `json:"plan_errors"`
	ProjectedMinor   int64                `json:"projected_balance_minor"`
	ReadyCount       int                  `json:"ready_count"`
	DuplicateCount   int                  `json:"duplicate_count"`
	InvalidCount     int                  `json:"invalid_count"`
	PlannedCount     int                  `json:"planned_count"`
	OtherSourceCount int                  `json:"other_source_count"`
}

// ImportReceipt describes the committed batch and where to review its records.
type ImportReceipt struct {
	ID                  int64     `json:"id"`
	AccountID           int64     `json:"account_id"`
	AccountName         string    `json:"account_name"`
	Currency            string    `json:"currency"`
	Filename            string    `json:"filename"`
	ImportedCount       int       `json:"imported_count"`
	SkippedCount        int       `json:"skipped_count"`
	InvalidCount        int       `json:"invalid_count"`
	DuplicateSkipped    int       `json:"duplicate_skipped"`
	BalanceBefore       int64     `json:"balance_before_minor"`
	BalanceAfter        int64     `json:"balance_after_minor"`
	ImportedMinor       int64     `json:"imported_minor"`
	StartingBefore      int64     `json:"starting_before_minor"`
	StartingAfter       int64     `json:"starting_after_minor"`
	NewCategories       int       `json:"new_categories"`
	TransactionIDs      []int64   `json:"transaction_ids"`
	PlannedIDs          []int64   `json:"planned_ids,omitempty"`
	PlannedCount        int       `json:"planned_count"`
	ExcludedReportCount int       `json:"excluded_report_count"`
	OtherSourceCount    int       `json:"other_source_count"`
	CompletedAt         time.Time `json:"completed_at"`
}

// ImportView returns either a reviewable plan or a completion receipt.
type ImportView struct {
	ID        int64          `json:"id"`
	AccountID int64          `json:"account_id"`
	Filename  string         `json:"filename"`
	Status    string         `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	ExpiresAt time.Time      `json:"expires_at"`
	Preview   *ImportPreview `json:"preview,omitempty"`
	Receipt   *ImportReceipt `json:"receipt,omitempty"`
}

// ImportService stages and atomically commits user-confirmed CSV records.
type ImportService struct {
	pool    *pgxpool.Pool
	q       *sqlc.Queries
	summary *SummaryService
}

// NewImportService shares the normal ledger writer and summary cache.
func NewImportService(pool *pgxpool.Pool, q *sqlc.Queries, summary *SummaryService) *ImportService {
	return &ImportService{pool: pool, q: q, summary: summary}
}

// Context returns account state without modifying it.
func (s *ImportService) Context(ctx context.Context, uid, accountID int64) (AccountLedgerContext, error) {
	return loadLedgerContext(ctx, s.q, uid, accountID)
}

func duplicateParams(rows []importing.Row, uid, account int64) sqlc.FindImportDuplicatesParams {
	p := sqlc.FindImportDuplicatesParams{UserID: uid, AccountID: account, RowNumbers: []int32{}, OccurredAts: []time.Time{}, Amounts: []int64{}, EntryTypes: []string{}, Fingerprints: []string{}, SourceRefs: []string{}}
	for _, r := range rows {
		if len(r.Errors) > 0 {
			continue
		}
		p.RowNumbers = append(p.RowNumbers, int32(r.Number)) // #nosec G115 -- server-parsed source line is bounded by the 16 MiB CSV limit.
		p.OccurredAts = append(p.OccurredAts, r.OccurredAt)
		p.Amounts = append(p.Amounts, r.AmountMinor)
		p.EntryTypes = append(p.EntryTypes, r.EntryType)
		p.Fingerprints = append(p.Fingerprints, r.Fingerprint)
		p.SourceRefs = append(p.SourceRefs, r.SourceRef)
	}
	return p
}

// Preview parses before taking the ledger lock, then stages a consistent plan.
func (s *ImportService) Preview(ctx context.Context, uid int64, in ImportInput) (ImportView, error) {
	doc, err := importing.Inspect(in.CSV, in.Delimiter)
	if err != nil {
		return ImportView{}, fmt.Errorf("%w: %s", ErrImportSelection, err)
	}
	sourceCount := doc.RowCount
	doc, err = importing.SelectSource(doc, in.SourceGroup)
	if err != nil {
		return ImportView{}, fmt.Errorf("%w: %s", ErrImportSelection, err)
	}
	if in.Starting.Mode != "existing" && in.Starting.Mode != "set" {
		return ImportView{}, ErrImportSelection
	}
	if !safeMinor(in.Starting.AmountMinor) {
		return ImportView{}, ErrMoneyRange
	}
	filename := strings.TrimSpace(in.Filename)
	if filename == "" {
		filename = "transactions.csv"
	}
	if len([]rune(filename)) > 200 {
		return ImportView{}, fmt.Errorf("%w: filename must be at most 200 characters", ErrImportSelection)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ImportView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return ImportView{}, err
	}
	account, err := loadLedgerContext(ctx, q, uid, in.AccountID)
	if err != nil {
		return ImportView{}, err
	}
	if !account.Active {
		return ImportView{}, ErrAccountInactive
	}
	rows, err := importing.Normalize(doc, in.Mapping, account.Currency, time.Now().UTC())
	if err != nil {
		return ImportView{}, fmt.Errorf("%w: %s", ErrImportSelection, err)
	}
	preview := ImportPreview{OtherSourceCount: sourceCount - doc.RowCount, Context: account, Rows: rows, Starting: in.Starting, Mapping: in.Mapping, NewCategories: []string{}, PlanErrors: []string{}}
	var start *time.Time
	if in.Starting.Mode == "set" {
		if account.OpeningCount > 1 {
			preview.PlanErrors = append(preview.PlanErrors, "Review multiple active starting-balance entries before replacing the starting balance.")
		}
		date := in.Starting.Date
		if date == "" {
			for _, row := range rows {
				if len(row.Errors) == 0 && (date == "" || row.OccurredAt.UTC().Format("2006-01-02") < date) {
					date = row.OccurredAt.UTC().Format("2006-01-02")
				}
			}
		}
		parsed, err := time.Parse("2006-01-02", date)
		if err != nil || parsed.Year() < 1 || parsed.After(time.Now().UTC()) {
			preview.PlanErrors = append(preview.PlanErrors, "Choose a valid starting date no later than today.")
		} else {
			start = &parsed
			preview.Starting.Date = date
		}
	} else {
		start = account.TrackingStartAt
		preview.Starting.AmountMinor = account.OpeningMinor
		if start != nil {
			preview.Starting.Date = start.UTC().Format("2006-01-02")
		}
	}
	for i := range preview.Rows {
		r := &preview.Rows[i]
		if len(r.Errors) == 0 && start != nil && r.OccurredAt.Before(*start) {
			r.Errors = append(r.Errors, "This record precedes the starting balance. Correct the starting date or leave the row out.")
		}
	}
	matches, err := findImportMatches(ctx, q, preview.Rows, uid, in.AccountID)
	if err != nil {
		return ImportView{}, err
	}
	for i := range preview.Rows {
		preview.Rows[i].Duplicates = append(preview.Rows[i].Duplicates, matches[preview.Rows[i].Number]...)
	}
	categories, err := q.ListCategoriesByUser(ctx, uid)
	if err != nil {
		return ImportView{}, err
	}
	known := map[string]string{}
	for _, c := range categories {
		known[strings.ToLower(c.Name)] = c.Name
	}
	newNames := map[string]bool{}
	projected := account.BalanceMinor
	if in.Starting.Mode == "set" {
		projected, err = addMinor(projected, -account.OpeningMinor)
		if err == nil {
			projected, err = addMinor(projected, in.Starting.AmountMinor)
		}
		if err != nil {
			return ImportView{}, err
		}
	}
	for i := range preview.Rows {
		r := &preview.Rows[i]
		if len(r.Errors) > 0 {
			preview.InvalidCount++
			continue
		}
		if r.Category != "" {
			key := strings.ToLower(r.Category)
			if name, ok := known[key]; ok {
				r.Category = name
			} else {
				r.NewCategory = true
				if !newNames[key] {
					preview.NewCategories = append(preview.NewCategories, r.Category)
					newNames[key] = true
				}
			}
		}
		if len(r.Duplicates) > 0 {
			preview.DuplicateCount++
			continue
		}
		preview.ReadyCount++
		if r.Planned {
			preview.PlannedCount++
			continue
		}
		projected, err = addMinor(projected, r.AmountMinor)
		if err != nil {
			return ImportView{}, err
		}
	}
	preview.ProjectedMinor = projected
	data, err := json.Marshal(preview)
	if err != nil {
		return ImportView{}, err
	}
	if err := q.DeleteExpiredImportPreviews(ctx, uid); err != nil {
		return ImportView{}, err
	}
	stage, err := q.CreateImportSession(ctx, sqlc.CreateImportSessionParams{UserID: uid, AccountID: in.AccountID, Filename: filename, FileHash: importing.Hash(in.CSV), Preview: data})
	if err != nil {
		return ImportView{}, err
	}
	if err := q.PruneImportPreviews(ctx, uid); err != nil {
		return ImportView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportView{}, err
	}
	return importView(stage)
}

func importView(stage sqlc.ImportSession) (ImportView, error) {
	v := ImportView{ID: stage.ID, AccountID: stage.AccountID, Filename: stage.Filename, Status: stage.Status, CreatedAt: stage.CreatedAt, ExpiresAt: stage.ExpiresAt}
	if stage.Status == "completed" {
		var receipt ImportReceipt
		if err := json.Unmarshal(stage.Result, &receipt); err != nil {
			return ImportView{}, err
		}
		v.Receipt = &receipt
	} else {
		if stage.ExpiresAt.Before(time.Now()) {
			return ImportView{}, ErrImportExpired
		}
		var preview ImportPreview
		if err := json.Unmarshal(stage.Preview, &preview); err != nil {
			return ImportView{}, err
		}
		v.Preview = &preview
	}
	return v, nil
}

// Get returns an owned preview/receipt, including after the browser is reloaded.
func (s *ImportService) Get(ctx context.Context, uid, id int64) (ImportView, error) {
	stage, err := s.q.GetImportSession(ctx, sqlc.GetImportSessionParams{ID: id, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportView{}, ErrImportNotFound
	}
	if err != nil {
		return ImportView{}, err
	}
	return importView(stage)
}

// ImportSelection is the exact set of rows a customer explicitly reviewed.
type ImportSelection struct {
	Rows               []int `json:"rows"`
	DuplicateApprovals []int `json:"duplicate_approvals"`
}

func canonicalSelection(in ImportSelection) (ImportSelection, error) {
	if len(in.Rows) > importing.MaxRows || len(in.DuplicateApprovals) > importing.MaxRows {
		return in, ErrImportSelection
	}
	in.Rows = append([]int{}, in.Rows...)
	in.DuplicateApprovals = append([]int{}, in.DuplicateApprovals...)
	sort.Ints(in.Rows)
	sort.Ints(in.DuplicateApprovals)
	for _, group := range [][]int{in.Rows, in.DuplicateApprovals} {
		for i, n := range group {
			if n < 1 || (i > 0 && group[i-1] == n) {
				return in, ErrImportSelection
			}
		}
	}
	return in, nil
}

// Commit creates every selected entry, category, origin, and outbox event in one
// transaction. The stage ID + canonical selection is its durable replay key.
func (s *ImportService) Commit(ctx context.Context, uid, id int64, selection ImportSelection) (ImportReceipt, error) {
	selection, err := canonicalSelection(selection)
	if err != nil {
		return ImportReceipt{}, err
	}
	hash := importing.Hash(selection)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ImportReceipt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return ImportReceipt{}, err
	}
	stage, err := q.GetImportSessionForUpdate(ctx, sqlc.GetImportSessionForUpdateParams{ID: id, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return ImportReceipt{}, ErrImportNotFound
	}
	if err != nil {
		return ImportReceipt{}, err
	}
	if stage.Status == "completed" {
		if stage.CommitHash.String != hash {
			return ImportReceipt{}, ErrImportCommitted
		}
		var receipt ImportReceipt
		err := json.Unmarshal(stage.Result, &receipt)
		return receipt, err
	}
	view, err := importView(stage)
	if err != nil {
		return ImportReceipt{}, err
	}
	preview := *view.Preview
	current, err := loadLedgerContext(ctx, q, uid, stage.AccountID)
	if err != nil {
		return ImportReceipt{}, err
	}
	if !current.Active {
		return ImportReceipt{}, ErrAccountInactive
	}
	if current.Token != preview.Context.Token {
		return ImportReceipt{}, ErrImportChanged
	}
	// An imported record can move to another owned account. Its duplicate
	// details can change without changing this destination's posting revision.
	oldMatches := map[int][]importing.Duplicate{}
	for _, row := range preview.Rows {
		for _, match := range row.Duplicates {
			if match.TransactionID != 0 || match.PlannedID != 0 {
				oldMatches[row.Number] = append(oldMatches[row.Number], match)
			}
		}
	}
	newMatches, err := findImportMatches(ctx, q, preview.Rows, uid, stage.AccountID)
	if err != nil {
		return ImportReceipt{}, err
	}
	if importing.Hash(oldMatches) != importing.Hash(newMatches) {
		return ImportReceipt{}, ErrImportChanged
	}
	if len(preview.PlanErrors) > 0 || (preview.Starting.Mode == "set" && len(selection.Rows) == 0) {
		return ImportReceipt{}, ErrImportSelection
	}
	selected, approved := map[int]bool{}, map[int]bool{}
	for _, n := range selection.Rows {
		selected[n] = true
	}
	for _, n := range selection.DuplicateApprovals {
		approved[n] = true
		if !selected[n] {
			return ImportReceipt{}, ErrImportSelection
		}
	}
	chosen := []importing.Row{}
	invalid, duplicateSkipped := 0, 0
	for _, r := range preview.Rows {
		if len(r.Errors) > 0 {
			invalid++
		}
		if !selected[r.Number] {
			if len(r.Duplicates) > 0 {
				duplicateSkipped++
			}
			continue
		}
		if len(r.Errors) > 0 || (len(r.Duplicates) > 0 && !approved[r.Number]) || (approved[r.Number] && len(r.Duplicates) == 0) {
			return ImportReceipt{}, ErrImportSelection
		}
		chosen = append(chosen, r)
	}
	if len(chosen) != len(selection.Rows) {
		return ImportReceipt{}, ErrImportSelection
	}
	receipt := ImportReceipt{OtherSourceCount: preview.OtherSourceCount, ID: id, AccountID: current.AccountID, AccountName: current.Name, Currency: current.Currency, Filename: stage.Filename, ImportedCount: len(chosen), SkippedCount: len(preview.Rows) - len(chosen), InvalidCount: invalid, DuplicateSkipped: duplicateSkipped, BalanceBefore: current.BalanceMinor, BalanceAfter: current.BalanceMinor, StartingBefore: current.OpeningMinor, StartingAfter: current.OpeningMinor, TransactionIDs: []int64{}, CompletedAt: time.Now().UTC()}
	var external int64
	if len(chosen) > 0 {
		external, err = q.EnsureExternalAccount(ctx, sqlc.EnsureExternalAccountParams{UserID: uid, Currency: current.Currency})
		if err != nil {
			return ImportReceipt{}, err
		}
	}
	var originalStart *time.Time
	if preview.Starting.Mode == "set" {
		start, err := time.Parse("2006-01-02", preview.Starting.Date)
		if err != nil {
			return ImportReceipt{}, ErrImportSelection
		}
		originalStart = current.OpeningAt
		if current.OpeningID != 0 {
			if _, _, _, err := appendReversal(ctx, q, uid, current.OpeningID); err != nil {
				return ImportReceipt{}, err
			}
		}
		if preview.Starting.AmountMinor != 0 {
			if _, err := appendLedger(ctx, q, ledgerWrite{UserID: uid, Currency: current.Currency, At: start, Note: "Starting balance for CSV import", EntryType: "opening_balance", ImportID: id, Legs: []Leg{{AccountID: current.AccountID, AmountMinor: preview.Starting.AmountMinor}, {AccountID: external, AmountMinor: -preview.Starting.AmountMinor}}}); err != nil {
				return ImportReceipt{}, err
			}
		}
		if err := q.SetTrackingStart(ctx, sqlc.SetTrackingStartParams{AccountID: current.AccountID, TrackingStartAt: pgtype.Timestamptz{Time: start, Valid: true}}); err != nil {
			return ImportReceipt{}, err
		}
		receipt.StartingAfter = preview.Starting.AmountMinor
		receipt.BalanceAfter, err = addMinor(receipt.BalanceAfter, -current.OpeningMinor)
		if err == nil {
			receipt.BalanceAfter, err = addMinor(receipt.BalanceAfter, preview.Starting.AmountMinor)
		}
		if err != nil {
			return ImportReceipt{}, err
		}
	}
	categoryIDs := map[string]int64{}
	known, err := q.ListCategoriesByUser(ctx, uid)
	if err != nil {
		return ImportReceipt{}, err
	}
	wasKnown := map[int64]bool{}
	for _, c := range known {
		wasKnown[c.ID] = true
	}
	copyRows := []sqlc.ImportCopyRow{}
	for _, row := range chosen {
		var categoryID int64
		if row.Category != "" {
			key := strings.ToLower(row.Category)
			categoryID = categoryIDs[key]
			if categoryID == 0 {
				category, err := q.EnsureImportCategory(ctx, sqlc.EnsureImportCategoryParams{UserID: uid, Name: row.Category})
				if err != nil {
					return ImportReceipt{}, err
				}
				categoryID = category.ID
				categoryIDs[key] = categoryID
				if !wasKnown[categoryID] {
					receipt.NewCategories++
					wasKnown[categoryID] = true
				}
			}
		}
		copyRows = append(copyRows, sqlc.ImportCopyRow{Row: row, CategoryID: categoryID})
		if row.ExcludeReport {
			receipt.ExcludedReportCount++
		}
		if row.Planned {
			receipt.PlannedCount++
			continue
		}
		receipt.ImportedMinor, err = addMinor(receipt.ImportedMinor, row.AmountMinor)
		if err == nil {
			receipt.BalanceAfter, err = addMinor(receipt.BalanceAfter, row.AmountMinor)
		}
		if err != nil {
			return ImportReceipt{}, err
		}
	}
	receipt.TransactionIDs, receipt.PlannedIDs, err = q.CopyImport(ctx, uid, current.AccountID, external, id, current.Currency, copyRows)
	if err != nil {
		return ImportReceipt{}, err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return ImportReceipt{}, err
	}
	if err := q.CompleteImportSession(ctx, sqlc.CompleteImportSessionParams{ID: id, UserID: uid, CommitHash: pgtype.Text{String: hash, Valid: true}, Result: data}); err != nil {
		return ImportReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ImportReceipt{}, err
	}
	// Imports never enqueue optional model tasks; category review stays explicit.
	months := map[string]time.Time{}
	for _, r := range chosen {
		at := utcMonthStart(r.OccurredAt)
		months[at.Format("2006-01")] = at
	}
	if preview.Starting.Mode == "set" {
		at, _ := time.Parse("2006-01-02", preview.Starting.Date)
		months[at.Format("2006-01")] = utcMonthStart(at)
		now := time.Now().UTC()
		months[now.Format("2006-01")] = utcMonthStart(now)
		if originalStart != nil {
			months[originalStart.Format("2006-01")] = utcMonthStart(*originalStart)
		}
	}
	if s.summary != nil {
		for _, at := range months {
			s.summary.Invalidate(ctx, current.AccountID, at)
			if external != 0 {
				s.summary.Invalidate(ctx, external, at)
			}
		}
	}
	return receipt, nil
}

// Combine posted and pending-plan matches under the same per-user write lock.
func findImportMatches(ctx context.Context, q *sqlc.Queries, rows []importing.Row, uid, account int64) (map[int][]importing.Duplicate, error) {
	p := duplicateParams(rows, uid, account)
	out := map[int][]importing.Duplicate{}
	posted, err := q.FindImportDuplicates(ctx, p)
	if err != nil {
		return nil, err
	}
	for _, m := range posted {
		out[int(m.RowNumber)] = append(out[int(m.RowNumber)], importing.Duplicate{TransactionID: m.TransactionID, OccurredAt: m.OccurredAt, AmountMinor: m.AmountMinor, Note: m.Note, MatchType: m.MatchType, AccountName: m.AccountName})
	}
	plans, err := q.FindPlannedImportDuplicates(ctx, sqlc.FindPlannedImportDuplicatesParams{RowNumbers: p.RowNumbers, OccurredAts: p.OccurredAts, Amounts: p.Amounts, Fingerprints: p.Fingerprints, SourceRefs: p.SourceRefs, UserID: uid, AccountID: account})
	if err != nil {
		return nil, err
	}
	for _, m := range plans {
		out[int(m.RowNumber)] = append(out[int(m.RowNumber)], importing.Duplicate{PlannedID: m.ID, OccurredAt: m.OccurredAt, AmountMinor: m.AmountMinor, Note: m.Note, MatchType: m.MatchType})
	}
	return out, nil
}
