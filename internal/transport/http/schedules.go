package httptransport

import (
	"net/http"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
)

// SchedulesAPI exposes recurring commitments and estimated cash flow.
type SchedulesAPI struct{ svc *service.ScheduleService }

// NewSchedulesAPI uses the existing ledger database and user locks.
func NewSchedulesAPI(s *service.ScheduleService) *SchedulesAPI { return &SchedulesAPI{s} }

// Routes registers owner-scoped recurring planning, never automatic posting.
func (a *SchedulesAPI) Routes(r chi.Router) {
	r.Get("/schedules", a.List)
	r.Post("/schedules", a.Create)
	r.Put("/schedules/{id}", a.Edit)
	r.Post("/schedules/{id}/{action}", a.Action)
	r.Post("/schedules/sync", a.Sync)
	r.Get("/planning/cashflow", a.Cashflow)
}

// List includes upcoming labels and the latest ended schedules.
func (a *SchedulesAPI) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	v, err := a.svc.List(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (a *SchedulesAPI) save(w http.ResponseWriter, r *http.Request, editing bool) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id := int64(0)
	if editing {
		id = categoryID(r)
		if id <= 0 {
			http.Error(w, "invalid schedule", 400)
			return
		}
	}
	var in service.ScheduleInput
	if err := readJSON(w, r, 8192, &in); err != nil {
		http.Error(w, "invalid schedule", 400)
		return
	}
	v, err := a.svc.Save(r.Context(), uid, id, r.Header.Get("Idempotency-Key"), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	status := 200
	if !editing {
		status = 201
	}
	writeJSON(w, status, v)
}

// Create saves one schedule and its initial pending occurrences atomically.
func (a *SchedulesAPI) Create(w http.ResponseWriter, r *http.Request) { a.save(w, r, false) }

// Edit changes pending occurrences from today; recorded and skipped dates remain.
func (a *SchedulesAPI) Edit(w http.ResponseWriter, r *http.Request) { a.save(w, r, true) }

// Action pauses, resumes or ends a reviewed schedule revision.
func (a *SchedulesAPI) Action(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	var in struct {
		Revision int64 `json:"revision"`
	}
	if err := readJSON(w, r, 1024, &in); err != nil {
		http.Error(w, "invalid schedule action", 400)
		return
	}
	v, err := a.svc.Action(r.Context(), uid, categoryID(r), in.Revision, chi.URLParam(r, "action"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, v)
}

// Sync refreshes pending occurrences without moving money.
func (a *SchedulesAPI) Sync(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	if err := a.svc.Sync(r.Context(), uid, time.Now()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"refreshed": true})
}

// Cashflow reads projected wallet balances from a coherent snapshot.
func (a *SchedulesAPI) Cashflow(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	v, err := a.svc.Cashflow(r.Context(), uid, time.Now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
