package httptransport

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/importing"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
)

// ImportsAPI serves CSV inspection, staged imports, and current balance checks.
type ImportsAPI struct {
	q      *sqlc.Queries
	svc    *service.ImportService
	checks *service.BalanceCheckService
}

// NewImportsAPI wires the user-confirmed data tools.
func NewImportsAPI(q *sqlc.Queries, svc *service.ImportService, checks *service.BalanceCheckService) *ImportsAPI {
	return &ImportsAPI{q: q, svc: svc, checks: checks}
}

// Routes keeps import/reconciliation tools secondary to the existing workspace.
func (a *ImportsAPI) Routes(r chi.Router) {
	r.Post("/imports/inspect", a.Inspect)
	r.Post("/imports/preview", a.Preview)
	r.Get("/imports", a.List)
	r.Get("/imports/{id}", a.Get)
	r.Post("/imports/{id}/commit", a.Commit)
	r.Get("/accounts/{id}/import-context", a.Context)
	r.Get("/accounts/{id}/balance-checks", a.CheckHistory)
	r.Post("/accounts/{id}/balance-checks", a.Check)
}

func importUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	uid, _ := r.Context().Value(UserIDKey).(int64)
	if uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return 0, false
	}
	w.Header().Set("Cache-Control", "no-store")
	return uid, true
}
func importID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// Inspect parses a file without persisting it or modifying any ledger.
func (a *ImportsAPI) Inspect(w http.ResponseWriter, r *http.Request) {
	if _, ok := importUser(w, r); !ok {
		return
	}
	var body struct {
		CSV       string `json:"csv"`
		Delimiter string `json:"delimiter"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*importing.MaxBytes)).Decode(&body); err != nil {
		http.Error(w, "invalid or oversized CSV request", http.StatusBadRequest)
		return
	}
	doc, err := importing.Inspect(body.CSV, body.Delimiter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// Context provides the current balance and explicit starting component.
func (a *ImportsAPI) Context(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	result, err := a.svc.Context(r.Context(), uid, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// Preview stages server-normalized rows for review; no money is posted.
func (a *ImportsAPI) Preview(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	var body service.ImportInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*importing.MaxBytes)).Decode(&body); err != nil {
		http.Error(w, "invalid or oversized preview request", http.StatusBadRequest)
		return
	}
	if body.AccountID <= 0 {
		http.Error(w, "choose an account", http.StatusUnprocessableEntity)
		return
	}
	result, err := a.svc.Preview(r.Context(), uid, body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// Get resumes an owned preview or returns its durable completion receipt.
func (a *ImportsAPI) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	result, err := a.svc.Get(r.Context(), uid, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// Commit posts only the rows explicitly chosen from the immutable preview.
func (a *ImportsAPI) Commit(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	var body service.ImportSelection
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 100*1024)).Decode(&body); err != nil {
		http.Error(w, "invalid selection", http.StatusBadRequest)
		return
	}
	result, err := a.svc.Commit(r.Context(), uid, id, body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// List returns recent import attempts and receipts for the authenticated user.
func (a *ImportsAPI) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	rows, err := a.q.ListImportSessions(r.Context(), uid)
	if err != nil {
		http.Error(w, "could not load imports", http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{"id": row.ID, "account_id": row.AccountID, "filename": row.Filename, "status": row.Status, "created_at": row.CreatedAt}
		if row.Status == "completed" {
			var receipt service.ImportReceipt
			if err := json.Unmarshal(row.Result, &receipt); err != nil {
				http.Error(w, "could not read import receipt", http.StatusInternalServerError)
				return
			}
			item["receipt"] = receipt
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

type balanceCheckDTO struct {
	ID              int64     `json:"id"`
	AccountID       int64     `json:"account_id"`
	ActualMinor     int64     `json:"actual_minor"`
	RecordedMinor   int64     `json:"recorded_minor"`
	DifferenceMinor int64     `json:"difference_minor"`
	Revision        int64     `json:"ledger_revision"`
	Note            string    `json:"note"`
	CreatedAt       time.Time `json:"created_at"`
}

func checkDTO(c sqlc.BalanceCheck) balanceCheckDTO {
	return balanceCheckDTO{ID: c.ID, AccountID: c.AccountID, ActualMinor: c.ActualMinor, RecordedMinor: c.RecordedMinor, DifferenceMinor: c.DifferenceMinor, Revision: c.LedgerRevision, Note: c.Note, CreatedAt: c.CreatedAt}
}

// Check stores a comparison with the user's posted bank/wallet balance.
func (a *ImportsAPI) Check(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	var body struct {
		ActualMinor *int64 `json:"actual_minor"`
		Note        string `json:"note"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil || body.ActualMinor == nil {
		http.Error(w, "enter the actual posted balance", http.StatusUnprocessableEntity)
		return
	}
	result, err := a.checks.Check(r.Context(), service.BalanceCheckInput{UserID: uid, AccountID: id, ActualMinor: *body.ActualMinor, Note: body.Note, Key: r.Header.Get("Idempotency-Key")})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, checkDTO(result))
}

// CheckHistory includes the current revision so old matches are not shown as fresh.
func (a *ImportsAPI) CheckHistory(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	checks, account, err := a.checks.History(r.Context(), uid, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]balanceCheckDTO, 0, len(checks))
	for _, c := range checks {
		out = append(out, checkDTO(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"checks": out, "current_revision": account.Revision, "recorded_balance_minor": account.BalanceMinor, "currency": account.Currency})
}
