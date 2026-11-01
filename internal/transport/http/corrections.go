package httptransport

import (
	"encoding/json"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
	"net/http"
)

// CorrectionsAPI serves explicit before/after reviews and atomic edits.
type CorrectionsAPI struct{ svc *service.CorrectionService }

// NewCorrectionsAPI creates the edit workflow handler.
func NewCorrectionsAPI(svc *service.CorrectionService) *CorrectionsAPI { return &CorrectionsAPI{svc} }

// Routes adds correction endpoints alongside existing transaction details.
func (a *CorrectionsAPI) Routes(r chi.Router) {
	r.Get("/transactions/{id}/edit-context", a.Context)
	r.Post("/transactions/{id}/correction-preview", a.Preview)
	r.Post("/transactions/{id}/corrections", a.Correct)
	r.Get("/transactions/{id}/corrections/result", a.Result)
}

// Result recovers a committed receipt for the same private write attempt.
func (a *CorrectionsAPI) Result(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	out, err := a.svc.Result(r.Context(), uid, id, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Context returns an owned original's editable fields and review token.
func (a *CorrectionsAPI) Context(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	out, err := a.svc.Context(r.Context(), uid, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func correctionBody(w http.ResponseWriter, r *http.Request) (service.CorrectionInput, bool) {
	var in service.CorrectionInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		http.Error(w, "check the correction fields", http.StatusBadRequest)
		return in, false
	}
	return in, true
}

// Preview calculates affected balances without posting any entries.
func (a *CorrectionsAPI) Preview(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	in, ok := correctionBody(w, r)
	if !ok {
		return
	}
	out, err := a.svc.Preview(r.Context(), uid, id, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Correct commits precisely the reviewed plan, with an Idempotency-Key.
func (a *CorrectionsAPI) Correct(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	in, ok := correctionBody(w, r)
	if !ok {
		return
	}
	out, err := a.svc.Correct(r.Context(), uid, id, r.Header.Get("Idempotency-Key"), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
