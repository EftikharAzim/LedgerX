package httptransport

import (
	"encoding/json"
	"golang.org/x/text/currency"
	"net/http"
	"strconv"
	"strings"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
)

// PlansAPI exposes owner-scoped planned activity.
type PlansAPI struct {
	q   *sqlc.Queries
	svc *service.PlanService
}

// NewPlansAPI uses the ledger service for confirmation and cancellation.
func NewPlansAPI(q *sqlc.Queries, svc *service.PlanService) *PlansAPI { return &PlansAPI{q, svc} }

// Routes registers planned activity endpoints.
func (a *PlansAPI) Routes(r chi.Router) {
	r.Get("/plans", a.List)
	r.Post("/plans", a.Create)
	r.Post("/plans/{id}/post", a.Post)
	r.Post("/plans/{id}/cancel", a.Cancel)
}

// List returns a bounded page of pending plans.
func (a *PlansAPI) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	offset := parseInt(r, "offset", 0)
	if offset < 0 || offset > 50000 {
		http.Error(w, "invalid page", 400)
		return
	}
	view, code := r.URL.Query().Get("view"), r.URL.Query().Get("currency")
	if view != "" && view != "all" && view != "overdue" && view != "soon" {
		http.Error(w, "choose all, overdue or soon", 400)
		return
	}
	if code != "" {
		if _, err := currency.ParseISO(code); err != nil || len(code) != 3 || code != strings.ToUpper(code) {
			http.Error(w, "choose a valid currency", 400)
			return
		}
	}
	rows, err := a.q.ListPlansForUser(r.Context(), sqlc.ListPlansForUserParams{UserID: uid, PageLimit: 51, PageOffset: int32(offset), View: view, CurrencyFilter: code})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	items := []map[string]any{}
	for i, p := range rows {
		if i >= 50 {
			break
		}
		items = append(items, map[string]any{"id": p.ID, "account_id": p.AccountID, "account_name": p.AccountName, "category_id": p.CategoryID.Int64, "category_name": p.CategoryName, "currency": p.Currency, "amount_minor": p.AmountMinor, "occurred_at": p.OccurredAt, "note": p.Note, "entry_type": p.EntryType, "date_only": p.DateOnly, "exclude_from_reports": p.ExcludeFromReports, "status": p.Status, "schedule_id": p.ScheduleID.Int64})
	}
	next := 0
	if len(rows) > 50 {
		next = offset + 50
	}
	writeJSON(w, 200, map[string]any{"entries": items, "next_offset": next})
}

// Create saves a future entry with a stable retry key.
func (a *PlansAPI) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	var body service.PlanInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		http.Error(w, "invalid plan", 400)
		return
	}
	p, err := a.svc.Create(r.Context(), uid, r.Header.Get("Idempotency-Key"), body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": p.ID, "status": p.Status, "planned": true})
}
func (a *PlansAPI) resolve(w http.ResponseWriter, r *http.Request, cancel bool) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid plan", 400)
		return
	}
	var body struct {
		Date        string `json:"date"`
		AmountMinor *int64 `json:"amount_minor"`
	}
	if !cancel {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
			http.Error(w, "choose the actual recorded date", 400)
			return
		}
	}
	tid, err := a.svc.ResolveAmount(r.Context(), uid, id, body.Date, cancel, body.AmountMinor)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "transaction_id": tid})
}

// Post confirms the actual date and records a plan once.
func (a *PlansAPI) Post(w http.ResponseWriter, r *http.Request) { a.resolve(w, r, false) }

// Cancel closes a plan without changing the ledger.
func (a *PlansAPI) Cancel(w http.ResponseWriter, r *http.Request) { a.resolve(w, r, true) }
