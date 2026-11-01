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
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Workspace recovery errors are safe to display in the restore workflow.
var (
	ErrWorkspaceNotEmpty = errors.New("restore needs an empty workspace; sign into a new LedgerX workspace and choose the backup there")
	ErrBackupInvalid     = errors.New("workspace backup is invalid")
	ErrRestoreNotFound   = errors.New("restore review not found")
	ErrRestoreExpired    = errors.New("this restore review expired; choose the backup again")
)

// WorkspaceService exports consistent data and restores reviewed files atomically.
type WorkspaceService struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewWorkspaceService uses the existing ledger database, without remote storage.
func NewWorkspaceService(pool *pgxpool.Pool, q *sqlc.Queries) *WorkspaceService {
	return &WorkspaceService{pool, q}
}

// RestoredAccount lets the browser restore optional account appearance locally.
type RestoredAccount struct {
	SourceID   int64                 `json:"source_id"`
	ID         int64                 `json:"id"`
	Name       string                `json:"name"`
	Currency   string                `json:"currency"`
	Appearance *workspace.Appearance `json:"appearance,omitempty"`
}

// RestoreReceipt describes one completed restore; its ID is a durable retry key.
type RestoreReceipt struct {
	ID          int64             `json:"id"`
	Filename    string            `json:"filename"`
	Checksum    string            `json:"checksum"`
	Accounts    []RestoredAccount `json:"accounts"`
	Summary     workspace.Summary `json:"summary"`
	CompletedAt time.Time         `json:"completed_at"`
}

// RestoreView exposes a summary/receipt, never the staged financial file itself.
type RestoreView struct {
	ID        int64             `json:"id"`
	Filename  string            `json:"filename"`
	Checksum  string            `json:"checksum"`
	Status    string            `json:"status"`
	Summary   workspace.Summary `json:"summary"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	Receipt   *RestoreReceipt   `json:"receipt,omitempty"`
}

// Backup reads authoritative records in one repeatable-read database snapshot.
func (s *WorkspaceService) Backup(ctx context.Context, uid int64, appearance map[int64]workspace.Appearance) (workspace.Backup, error) {
	var b workspace.Backup
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return b, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	b.Data, err = s.q.WithTx(tx).ReadWorkspaceData(ctx, uid)
	if err != nil {
		if errors.Is(err, workspace.ErrLimits) {
			return b, fmt.Errorf("%w: %s", ErrBackupInvalid, err)
		}
		return b, err
	}
	for i := range b.Data.Accounts {
		a := &b.Data.Accounts[i]
		if value, ok := appearance[a.ID]; ok && a.Kind == "normal" {
			appearanceCopy := value
			a.Appearance = &appearanceCopy
		}
	}
	b.Format = "ledgerx.workspace"
	b.Version = 1
	b.ExportedAt = time.Now().UTC()
	if err := b.Seal(); err != nil {
		return b, err
	}
	if _, err := validateWorkspaceBackup(b); err != nil {
		return b, err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return b, err
	}
	if len(raw) > workspace.MaxBytes {
		return b, fmt.Errorf("%w: file exceeds 32 MiB", ErrBackupInvalid)
	}
	return b, tx.Commit(ctx)
}

func validateWorkspaceBackup(b workspace.Backup) (workspace.Summary, error) {
	summary, err := b.Validate()
	if err != nil {
		return summary, fmt.Errorf("%w: %s", ErrBackupInvalid, err)
	}
	invalid := func(reason string) (workspace.Summary, error) {
		return summary, fmt.Errorf("%w: %s", ErrBackupInvalid, reason)
	}
	accounts := map[int64]workspace.Account{}
	categories := map[int64]bool{}
	for _, category := range b.Data.Categories {
		categories[category.ID] = true
	}
	txs := map[int64]workspace.Transaction{}
	originByImport := map[int64]map[int64]bool{}
	normalAmount := map[int64]map[int64]int64{}
	for _, a := range b.Data.Accounts {
		accounts[a.ID] = a
	}
	for _, t := range b.Data.Transactions {
		txs[t.ID] = t
	}
	for _, o := range b.Data.Origins {
		if originByImport[o.ImportID] == nil {
			originByImport[o.ImportID] = map[int64]bool{}
		}
		originByImport[o.ImportID][o.TransactionID] = true
	}
	for _, p := range b.Data.Postings {
		if normalAmount[p.TransactionID] == nil {
			normalAmount[p.TransactionID] = map[int64]int64{}
		}
		normalAmount[p.TransactionID][p.AccountID] += p.AmountMinor
	}
	planByID := map[int64]workspace.Plan{}
	for _, p := range b.Data.Plans {
		planByID[p.ID] = p
	}
	for _, i := range b.Data.Imports {
		var r ImportReceipt
		if err := json.Unmarshal(i.Receipt, &r); err != nil {
			return invalid("cannot read an import receipt")
		}
		if r.ID != i.ID || r.AccountID != i.AccountID || r.Currency != accounts[i.AccountID].Currency || r.Filename != i.Filename || r.ImportedCount != len(r.TransactionIDs)+len(r.PlannedIDs) || r.PlannedCount != len(r.PlannedIDs) || r.SkippedCount < 0 || r.InvalidCount < 0 || r.DuplicateSkipped < 0 || r.NewCategories < 0 || r.ImportedCount+r.SkippedCount+r.OtherSourceCount > importing.MaxRows || r.OtherSourceCount < 0 || r.ExcludedReportCount < 0 || r.ExcludedReportCount > r.ImportedCount || r.InvalidCount > r.SkippedCount || r.DuplicateSkipped > r.SkippedCount || !safeMinor(r.BalanceBefore) || !safeMinor(r.BalanceAfter) || !safeMinor(r.ImportedMinor) || !safeMinor(r.StartingBefore) || !safeMinor(r.StartingAfter) || r.BalanceAfter-r.BalanceBefore != r.ImportedMinor+r.StartingAfter-r.StartingBefore {
			return invalid("an import receipt is inconsistent with its source records")
		}
		amount := int64(0)
		seen := map[int64]bool{}
		for _, id := range r.TransactionIDs {
			if seen[id] || !originByImport[i.ID][id] {
				return invalid("an import receipt refers to unknown source records")
			}
			seen[id] = true
			amount += normalAmount[id][r.AccountID]
		}
		seenPlans := map[int64]bool{}
		expectedOrigins := len(r.TransactionIDs)
		for _, id := range r.PlannedIDs {
			p, ok := planByID[id]
			if !ok || seenPlans[id] || p.ImportID != i.ID || p.AccountID != i.AccountID {
				return invalid("an import receipt refers to an unknown plan")
			}
			seenPlans[id] = true
			if p.Status == "posted" {
				if !originByImport[i.ID][p.TransactionID] {
					return invalid("a posted plan is missing its import origin")
				}
				expectedOrigins++
			}
		}
		if len(originByImport[i.ID]) != expectedOrigins {
			return invalid("import origins do not match posted records and plans")
		}
		if amount != r.ImportedMinor {
			return invalid("import receipt amount does not match its postings")
		}
	}
	for _, c := range b.Data.Corrections {
		var r CorrectionReceipt
		if err := json.Unmarshal(c.Receipt, &r); err != nil {
			return invalid("cannot read correction history")
		}
		if r.OriginalID != c.OriginalID || r.ReversalID != c.ReversalID || r.ReplacementID != c.ReplacementID || r.CompletedAt.IsZero() {
			return invalid("correction receipt links are inconsistent")
		}
		for _, d := range []CorrectionDraft{r.Old, r.New} {
			if accounts[d.AccountID].Kind != "normal" || accounts[d.AccountID].Currency != d.Currency || !safeMinor(d.AmountMinor) || (d.CategoryID != 0 && !categories[d.CategoryID]) || (d.ToAccountID != 0 && (accounts[d.ToAccountID].Kind != "normal" || accounts[d.ToAccountID].Currency != d.Currency)) {
				return invalid("correction receipt refers to an unknown account")
			}
		}
		original, replacement := txs[c.OriginalID], txs[c.ReplacementID]
		if !r.Old.OccurredAt.Equal(original.OccurredAt) || normalAmount[original.ID][r.Old.AccountID] != r.Old.AmountMinor || (c.ReplacementID != 0 && (!r.New.OccurredAt.Equal(replacement.OccurredAt) || normalAmount[replacement.ID][r.New.AccountID] != r.New.AmountMinor)) || (c.ReplacementID == 0 && (r.New.EntryType != "opening_balance" || r.New.AmountMinor != 0)) {
			return invalid("correction receipt money/date differs from the ledger")
		}
		delta := map[int64]int64{}
		for aid, amount := range normalAmount[original.ID] {
			if accounts[aid].Kind == "normal" {
				delta[aid] -= amount
			}
		}
		for aid, amount := range normalAmount[replacement.ID] {
			if accounts[aid].Kind == "normal" {
				delta[aid] += amount
			}
		}
		if _, ok := delta[r.New.AccountID]; !ok {
			delta[r.New.AccountID] = 0
		}
		seen := map[int64]bool{}
		for _, balance := range r.Balances {
			if seen[balance.AccountID] || accounts[balance.AccountID].Kind != "normal" || !safeMinor(balance.BeforeMinor) || !safeMinor(balance.AfterMinor) || !safeMinor(balance.ChangeMinor) || balance.AfterMinor-balance.BeforeMinor != balance.ChangeMinor || delta[balance.AccountID] != balance.ChangeMinor || balance.Currency != accounts[balance.AccountID].Currency {
				return invalid("correction balance review is inconsistent")
			}
			seen[balance.AccountID] = true
		}
		if len(seen) != len(delta) {
			return invalid("correction balance review is incomplete")
		}
	}
	return summary, nil
}

// Preview validates a file and persists only an owner-scoped, expiring review.
func (s *WorkspaceService) Preview(ctx context.Context, uid int64, filename string, raw []byte) (RestoreView, error) {
	var empty RestoreView
	b, err := workspace.Decode(raw)
	if err != nil {
		return empty, fmt.Errorf("%w: %s", ErrBackupInvalid, err)
	}
	summary, err := validateWorkspaceBackup(b)
	if err != nil {
		return empty, err
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "workspace.ledgerx.json"
	}
	if len([]rune(filename)) > 200 || strings.ContainsRune(filename, 0) {
		return empty, fmt.Errorf("%w: filename is too long", ErrBackupInvalid)
	}
	payload, err := json.Marshal(b)
	if err != nil {
		return empty, err
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return empty, err
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
	row, err := q.CreateRestorePreview(ctx, sqlc.CreateRestorePreviewParams{UserID: uid, Filename: filename, Checksum: b.SHA256, Payload: payload, Summary: summaryJSON})
	if err != nil {
		return empty, err
	}
	if err := q.PruneRestorePreviews(ctx, uid); err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return restoreView(row)
}

func restoreView(row sqlc.WorkspaceRestore) (RestoreView, error) {
	out := RestoreView{ID: row.ID, Filename: row.Filename, Checksum: row.Checksum, Status: row.Status, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}
	if err := json.Unmarshal(row.Summary, &out.Summary); err != nil {
		return out, err
	}
	if row.Status == "completed" {
		out.Receipt = new(RestoreReceipt)
		if err := json.Unmarshal(row.Result, out.Receipt); err != nil {
			return out, err
		}
	} else if !time.Now().UTC().Before(row.ExpiresAt) {
		return out, ErrRestoreExpired
	}
	return out, nil
}

// Get resumes a review or a completed restore receipt without exposing payloads.
func (s *WorkspaceService) Get(ctx context.Context, uid, id int64) (RestoreView, error) {
	row, err := s.q.GetRestoreSession(ctx, sqlc.GetRestoreSessionParams{ID: id, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return RestoreView{}, ErrRestoreNotFound
	}
	if err != nil {
		return RestoreView{}, err
	}
	return restoreView(row)
}

type workspaceIDs struct{ accounts, categories, transactions, postings, imports, origins, corrections, checks, plans, budgets map[int64]int64 }

func reserveIDs(ctx context.Context, tx pgx.Tx, sequence string, source []int64) (map[int64]int64, error) {
	queries := map[string]string{
		"accounts": "SELECT nextval('accounts_id_seq') FROM generate_series(1,$1)", "categories": "SELECT nextval('categories_id_seq') FROM generate_series(1,$1)",
		"transactions": "SELECT nextval('transactions_id_seq') FROM generate_series(1,$1)", "postings": "SELECT nextval('postings_id_seq') FROM generate_series(1,$1)",
		"imports": "SELECT nextval('import_sessions_id_seq') FROM generate_series(1,$1)", "origins": "SELECT nextval('imported_transactions_id_seq') FROM generate_series(1,$1)",
		"corrections": "SELECT nextval('transaction_corrections_id_seq') FROM generate_series(1,$1)", "plans": "SELECT nextval('planned_transactions_id_seq') FROM generate_series(1,$1::integer)",
		"budgets": "SELECT nextval('budgets_id_seq') FROM generate_series(1,$1)",
		"checks":  "SELECT nextval('balance_checks_id_seq') FROM generate_series(1,$1)"}
	query, ok := queries[sequence]
	if !ok {
		return nil, errors.New("unknown workspace sequence")
	}
	out := map[int64]int64{}
	if len(source) == 0 {
		return out, nil
	}
	sort.Slice(source, func(i, j int) bool { return source[i] < source[j] })
	rows, err := tx.Query(ctx, query, len(source))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var next int64
		if err := rows.Scan(&next); err != nil {
			return out, err
		}
		if index >= len(source) {
			return out, errors.New("invalid workspace sequence count")
		}
		out[source[index]] = next
		index++
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if index != len(source) {
		return out, errors.New("incomplete workspace ID allocation")
	}
	return out, nil
}

func allocateWorkspace(ctx context.Context, tx pgx.Tx, q *sqlc.Queries, uid int64, d workspace.Data) (workspaceIDs, int64, error) {
	var maps workspaceIDs
	existing, err := q.GetExternalAccount(ctx, uid)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return maps, 0, err
	}
	existingID := existing.ID
	groups := map[string][]int64{"accounts": {}, "categories": {}, "transactions": {}, "postings": {}, "imports": {}, "origins": {}, "corrections": {}, "checks": {}, "plans": {}, "budgets": {}}
	var sourceExternal int64
	for _, v := range d.Accounts {
		if v.Kind == "external" && existingID != 0 {
			sourceExternal = v.ID
		} else {
			groups["accounts"] = append(groups["accounts"], v.ID)
		}
	}
	for _, v := range d.Categories {
		groups["categories"] = append(groups["categories"], v.ID)
	}
	for _, v := range d.Transactions {
		groups["transactions"] = append(groups["transactions"], v.ID)
	}
	for _, v := range d.Postings {
		groups["postings"] = append(groups["postings"], v.ID)
	}
	for _, v := range d.Imports {
		groups["imports"] = append(groups["imports"], v.ID)
	}
	for _, v := range d.Origins {
		groups["origins"] = append(groups["origins"], v.ID)
	}
	for _, v := range d.Corrections {
		groups["corrections"] = append(groups["corrections"], v.ID)
	}
	for _, v := range d.Plans {
		groups["plans"] = append(groups["plans"], v.ID)
	}
	for _, v := range d.Checks {
		groups["checks"] = append(groups["checks"], v.ID)
	}
	for _, v := range d.Budgets {
		groups["budgets"] = append(groups["budgets"], v.ID)
	}
	destinations := map[string]*map[int64]int64{"accounts": &maps.accounts, "categories": &maps.categories, "transactions": &maps.transactions, "postings": &maps.postings, "imports": &maps.imports, "origins": &maps.origins, "corrections": &maps.corrections, "checks": &maps.checks, "plans": &maps.plans, "budgets": &maps.budgets}
	for _, name := range []string{"accounts", "categories", "transactions", "postings", "imports", "origins", "corrections", "checks", "plans", "budgets"} {
		m, err := reserveIDs(ctx, tx, name, groups[name])
		if err != nil {
			return maps, 0, err
		}
		*destinations[name] = m
	}
	if sourceExternal != 0 {
		maps.accounts[sourceExternal] = existingID
	}
	return maps, sourceExternal, nil
}

func remapDraft(d CorrectionDraft, m workspaceIDs) CorrectionDraft {
	d.AccountID = m.accounts[d.AccountID]
	d.ToAccountID = m.accounts[d.ToAccountID]
	d.CategoryID = m.categories[d.CategoryID]
	return d
}

func remapWorkspace(d workspace.Data, m workspaceIDs) (workspace.Data, error) {
	for i := range d.Accounts {
		d.Accounts[i].ID = m.accounts[d.Accounts[i].ID]
	}
	for i := range d.Categories {
		d.Categories[i].ID = m.categories[d.Categories[i].ID]
		d.Categories[i].ParentID = m.categories[d.Categories[i].ParentID]
		d.Categories[i].ReplacementID = m.categories[d.Categories[i].ReplacementID]
	}
	for i := range d.Budgets {
		v := &d.Budgets[i]
		v.ID = m.budgets[v.ID]
		v.CategoryID = m.categories[v.CategoryID]
	}
	for i := range d.Transactions {
		v := &d.Transactions[i]
		v.ID = m.transactions[v.ID]
		v.CategoryID = m.categories[v.CategoryID]
		v.SuggestedCategoryID = m.categories[v.SuggestedCategoryID]
		v.ReversalOf = m.transactions[v.ReversalOf]
	}
	for i := range d.Plans {
		v := &d.Plans[i]
		v.ID = m.plans[v.ID]
		v.AccountID = m.accounts[v.AccountID]
		v.CategoryID = m.categories[v.CategoryID]
		v.ImportID = m.imports[v.ImportID]
		v.TransactionID = m.transactions[v.TransactionID]
	}
	for i := range d.Postings {
		v := &d.Postings[i]
		v.ID = m.postings[v.ID]
		v.TransactionID = m.transactions[v.TransactionID]
		v.AccountID = m.accounts[v.AccountID]
	}
	for i := range d.Imports {
		v := &d.Imports[i]
		var r ImportReceipt
		if err := json.Unmarshal(v.Receipt, &r); err != nil {
			return d, err
		}
		v.ID = m.imports[v.ID]
		v.AccountID = m.accounts[v.AccountID]
		r.ID = v.ID
		r.AccountID = v.AccountID
		for j := range r.TransactionIDs {
			r.TransactionIDs[j] = m.transactions[r.TransactionIDs[j]]
		}
		for j := range r.PlannedIDs {
			r.PlannedIDs[j] = m.plans[r.PlannedIDs[j]]
		}
		raw, err := json.Marshal(r)
		if err != nil {
			return d, err
		}
		v.Receipt = raw
	}
	for i := range d.Origins {
		v := &d.Origins[i]
		v.ID = m.origins[v.ID]
		v.TransactionID = m.transactions[v.TransactionID]
		v.AccountID = m.accounts[v.AccountID]
		v.ImportID = m.imports[v.ImportID]
	}
	for i := range d.Corrections {
		v := &d.Corrections[i]
		var r CorrectionReceipt
		if err := json.Unmarshal(v.Receipt, &r); err != nil {
			return d, err
		}
		v.ID = m.corrections[v.ID]
		v.OriginalID = m.transactions[v.OriginalID]
		v.ReversalID = m.transactions[v.ReversalID]
		v.ReplacementID = m.transactions[v.ReplacementID]
		v.ImportSourceID = m.transactions[v.ImportSourceID]
		r.OriginalID = v.OriginalID
		r.ReversalID = v.ReversalID
		r.ReplacementID = v.ReplacementID
		r.Old = remapDraft(r.Old, m)
		r.New = remapDraft(r.New, m)
		for j := range r.Balances {
			r.Balances[j].AccountID = m.accounts[r.Balances[j].AccountID]
		}
		raw, err := json.Marshal(r)
		if err != nil {
			return d, err
		}
		v.Receipt = raw
	}
	for i := range d.Checks {
		v := &d.Checks[i]
		v.ID = m.checks[v.ID]
		v.AccountID = m.accounts[v.AccountID]
		v.LedgerRevision = m.postings[v.LedgerRevision]
	}
	return d, nil
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func copyWorkspace(ctx context.Context, tx pgx.Tx, uid int64, d workspace.Data, externalID int64) error {
	copyRows := func(table string, columns []string, rows [][]any) error {
		if len(rows) == 0 {
			return nil
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{table}, columns, pgx.CopyFromRows(rows))
		return err
	}
	rows := [][]any{}
	for _, a := range d.Accounts {
		if a.ID == externalID {
			if _, err := tx.Exec(ctx, `UPDATE accounts SET name=$3,currency=$4,is_active=$5,created_at=$6,tracking_start_at=$7 WHERE id=$1 AND user_id=$2 AND kind='external'`, a.ID, uid, a.Name, a.Currency, a.Active, a.CreatedAt, a.TrackingStartAt); err != nil {
				return err
			}
			continue
		}
		rows = append(rows, []any{a.ID, uid, a.Name, a.Currency, a.Active, a.Kind, a.CreatedAt, a.TrackingStartAt})
	}
	if err := copyRows("accounts", []string{"id", "user_id", "name", "currency", "is_active", "kind", "created_at", "tracking_start_at"}, rows); err != nil {
		return err
	}
	rows = [][]any{}
	for _, v := range d.Categories {
		icon, kind := v.Icon, v.Kind
		if icon == "" {
			icon = "tag"
		}
		if kind == "" {
			kind = "any"
		}
		rows = append(rows, []any{v.ID, uid, v.Name, v.CreatedAt, nullableID(v.ParentID), icon, kind, v.Archived, v.Deleted, v.DeletedAt, nullableID(v.ReplacementID)})
	}
	if err := copyRows("categories", []string{"id", "user_id", "name", "created_at", "parent_id", "icon", "kind", "archived", "deleted", "deleted_at", "replacement_id"}, rows); err != nil {
		return err
	}
	rows = [][]any{}
	for _, v := range d.Budgets {
		at, _ := time.Parse("2006-01", v.Month)
		rows = append(rows, []any{v.ID, uid, v.Currency, at, nullableID(v.CategoryID), v.AmountMinor, v.CreatedAt, v.UpdatedAt})
	}
	if err := copyRows("budgets", []string{"id", "user_id", "currency", "month", "category_id", "amount_minor", "created_at", "updated_at"}, rows); err != nil {
		return err
	}
	rows = [][]any{}
	for _, v := range d.Transactions {
		rows = append(rows, []any{v.ID, uid, v.Currency, nullableID(v.CategoryID), nullableID(v.SuggestedCategoryID), v.SuggestionScore, v.OccurredAt, v.CreatedAt, v.Note, nullableID(v.ReversalOf), v.EntryType, v.ExcludeReport})
	}
	if err := copyRows("transactions", []string{"id", "user_id", "currency", "category_id", "suggested_category_id", "suggestion_score", "occurred_at", "created_at", "note", "reversal_of", "entry_type", "exclude_from_reports"}, rows); err != nil {
		return err
	}
	rows = [][]any{}
	sort.Slice(d.Postings, func(i, j int) bool { return d.Postings[i].ID < d.Postings[j].ID })
	for _, v := range d.Postings {
		rows = append(rows, []any{v.ID, v.TransactionID, v.AccountID, v.AmountMinor, v.CreatedAt})
	}
	if err := copyRows("postings", []string{"id", "transaction_id", "account_id", "amount_minor", "created_at"}, rows); err != nil {
		return err
	}
	rows = [][]any{}
	for _, v := range d.Imports {
		rows = append(rows, []any{v.ID, uid, v.AccountID, v.Filename, v.FileHash, "completed", json.RawMessage(`{}`), "restored", v.Receipt, v.CreatedAt, v.CompletedAt.Add(24 * time.Hour), v.CompletedAt})
	}
	if err := copyRows("import_sessions", []string{"id", "user_id", "account_id", "filename", "file_hash", "status", "preview", "commit_hash", "result", "created_at", "expires_at", "completed_at"}, rows); err != nil {
		return err
	}
	rows = [][]any{}
	for _, v := range d.Plans {
		rows = append(rows, []any{v.ID, uid, v.AccountID, v.Currency, v.AmountMinor, nullableID(v.CategoryID), v.OccurredAt, v.Note, v.EntryType, v.DateOnly, v.ExcludeReport, v.Status, nullableID(v.TransactionID), nullableID(v.ImportID), func() any {
			if v.RowNumber == 0 {
				return nil
			}
			return v.RowNumber
		}(), v.Fingerprint, v.SourceRef, v.CreatedAt})
	}
	if err := copyRows("planned_transactions", []string{"id", "user_id", "account_id", "currency", "amount_minor", "category_id", "occurred_at", "note", "entry_type", "date_only", "exclude_from_reports", "status", "transaction_id", "import_id", "row_number", "fingerprint", "source_ref", "created_at"}, rows); err != nil {
		return err
	}
	rows = [][]any{}
	for _, v := range d.Origins {
		rows = append(rows, []any{v.ID, uid, v.AccountID, v.ImportID, v.TransactionID, v.RowNumber, v.Fingerprint, v.SourceRef, v.DateOnly})
	}
	if err := copyRows("imported_transactions", []string{"id", "user_id", "account_id", "import_id", "transaction_id", "row_number", "fingerprint", "source_ref", "date_only"}, rows); err != nil {
		return err
	}
	rows = [][]any{}
	for _, v := range d.Corrections {
		rows = append(rows, []any{v.ID, uid, v.OriginalID, v.ReversalID, nullableID(v.ReplacementID), nullableID(v.ImportSourceID), v.DateOnly, fmt.Sprintf("restored-correction-%d", v.ID), "restored", v.Receipt, v.CreatedAt})
	}
	if err := copyRows("transaction_corrections", []string{"id", "user_id", "original_id", "reversal_id", "replacement_id", "import_source_id", "date_only", "request_key", "request_hash", "receipt", "created_at"}, rows); err != nil {
		return err
	}
	rows = [][]any{}
	for _, v := range d.Checks {
		rows = append(rows, []any{v.ID, uid, v.AccountID, v.ActualMinor, v.RecordedMinor, v.DifferenceMinor, v.LedgerRevision, v.Note, v.CreatedAt})
	}
	return copyRows("balance_checks", []string{"id", "user_id", "account_id", "actual_minor", "recorded_minor", "difference_minor", "ledger_revision", "note", "created_at"}, rows)
}

// Restore commits a reviewed backup to an empty workspace, or replays its receipt.
func (s *WorkspaceService) Restore(ctx context.Context, uid, id int64, checksum string) (RestoreReceipt, error) {
	var empty RestoreReceipt
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	if err := q.LockUserLedger(ctx, uid); err != nil {
		return empty, err
	}
	row, err := q.GetRestoreSessionForUpdate(ctx, sqlc.GetRestoreSessionForUpdateParams{ID: id, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, ErrRestoreNotFound
	}
	if err != nil {
		return empty, err
	}
	if checksum == "" || checksum != row.Checksum {
		return empty, ErrBackupInvalid
	}
	view, err := restoreView(row)
	if err != nil {
		return empty, err
	}
	if view.Receipt != nil {
		return *view.Receipt, nil
	}
	available, err := q.WorkspaceIsEmpty(ctx, uid)
	if err != nil {
		return empty, err
	}
	if !available.Valid || !available.Bool {
		return empty, ErrWorkspaceNotEmpty
	}
	b, err := workspace.Decode(row.Payload)
	if err != nil {
		return empty, err
	}
	if b.SHA256 != row.Checksum {
		return empty, ErrBackupInvalid
	}
	summary, err := validateWorkspaceBackup(b)
	if err != nil {
		return empty, err
	}
	// A fresh login can already have a derived snapshot for its empty system
	// account. Replace that empty account too: every restored posting then uses
	// a new account ID, so old snapshot jobs/cutoffs cannot hide restored history.
	if _, err := tx.Exec(ctx, `DELETE FROM accounts WHERE user_id=$1 AND kind='external' AND NOT EXISTS(SELECT 1 FROM postings p WHERE p.account_id=accounts.id)`, uid); err != nil {
		return empty, err
	}
	maps, sourceExternal, err := allocateWorkspace(ctx, tx, q, uid, b.Data)
	if err != nil {
		return empty, err
	}
	receipt := RestoreReceipt{ID: id, Filename: row.Filename, Checksum: checksum, Summary: summary, Accounts: []RestoredAccount{}, CompletedAt: time.Now().UTC()}
	for _, a := range b.Data.Accounts {
		if a.Kind == "normal" {
			receipt.Accounts = append(receipt.Accounts, RestoredAccount{SourceID: a.ID, ID: maps.accounts[a.ID], Name: a.Name, Currency: a.Currency, Appearance: a.Appearance})
		}
	}
	restored, err := remapWorkspace(b.Data, maps)
	if err != nil {
		return empty, err
	}
	if err := copyWorkspace(ctx, tx, uid, restored, maps.accounts[sourceExternal]); err != nil {
		return empty, err
	}
	result, err := json.Marshal(receipt)
	if err != nil {
		return empty, err
	}
	if err := q.CompleteRestore(ctx, sqlc.CompleteRestoreParams{ID: id, UserID: uid, Result: result}); err != nil {
		return empty, err
	}
	event, err := json.Marshal(map[string]any{"user_id": uid, "restore_id": id, "transaction_count": summary.TransactionCount})
	if err != nil {
		return empty, err
	}
	if _, err := q.CreateOutboxEvent(ctx, sqlc.CreateOutboxEventParams{EventType: "WorkspaceRestored", Payload: event}); err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return receipt, nil
}
