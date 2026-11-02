// Package httptransport serves the ledger API and its HTTP middleware.
package httptransport

import (
	"math"
	"net/http"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
)

// AccountsAPI handles financial account management for the authenticated user.
type AccountsAPI struct {
	q   *sqlc.Queries
	svc *service.AccountService
}

// NewAccountsAPI creates account handlers using the supplied queries.
func NewAccountsAPI(q *sqlc.Queries, svc *service.AccountService) *AccountsAPI {
	return &AccountsAPI{q: q, svc: svc}
}

type createAccountReq struct {
	Name         string    `json:"name"`
	Currency     string    `json:"currency"` // e.g., "USD"
	OpeningMinor int64     `json:"opening_balance_minor"`
	OpeningAt    time.Time `json:"opening_balance_at"`
}
type accountDTO struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Active   bool   `json:"active"`
	Kind     string `json:"kind"`
}

// Routes registers the account endpoints.
func (a *AccountsAPI) Routes(r chi.Router) {
	r.Post("/accounts", a.Create)
	r.Get("/accounts", a.List)
	r.Patch("/accounts/{id}", a.Update)
}

// Update renames, archives or restores an owned financial account.
func (a *AccountsAPI) Update(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	var body service.AccountEdit
	if err := readStrictJSON(w, r, 4096, &body); err != nil {
		http.Error(w, "invalid account update", http.StatusBadRequest)
		return
	}
	row, err := a.svc.Update(r.Context(), uid, id, body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accountDTO{ID: row.ID, UserID: row.UserID, Name: row.Name, Currency: row.Currency, Active: row.IsActive, Kind: row.Kind})
}

// Create adds an account owned by the authenticated user.
func (a *AccountsAPI) Create(w http.ResponseWriter, r *http.Request) {
	var req createAccountReq
	if err := readJSON(w, r, 8192, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if req.Name == "" {
		http.Error(w, "name required", http.StatusUnprocessableEntity)
		return
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}

	row, err := a.svc.Create(r.Context(), service.AccountInput{
		UserID: uid, Name: req.Name, Currency: req.Currency, OpeningMinor: req.OpeningMinor, OpeningAt: req.OpeningAt, IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, accountDTO{
		ID: row.ID, UserID: row.UserID, Name: row.Name, Currency: row.Currency,
		Active: row.IsActive, Kind: row.Kind,
	})
}

// List returns the authenticated user's accounts with limit and offset pagination.
func (a *AccountsAPI) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	limit := parseInt(r, "limit", 20)
	offset := parseInt(r, "offset", 0)
	if limit < 1 || limit > 100 {
		limit = 100
	}
	if offset < 0 || offset > math.MaxInt32 {
		http.Error(w, "offset is too large", http.StatusBadRequest)
		return
	}

	rows, err := a.q.ListAccountsByUser(r.Context(), sqlc.ListAccountsByUserParams{
		UserID: uid,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	out := make([]accountDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, accountDTO{
			ID: row.ID, UserID: row.UserID, Name: row.Name, Currency: row.Currency,
			Active: row.IsActive, Kind: row.Kind,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
