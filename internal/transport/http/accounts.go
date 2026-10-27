// Package httptransport serves the ledger API and its HTTP middleware.
package httptransport

import (
	"encoding/json"
	"math"
	"net/http"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/go-chi/chi/v5"
)

// AccountsAPI handles account creation and listing for the authenticated user.
type AccountsAPI struct{ q *sqlc.Queries }

// NewAccountsAPI creates account handlers using the supplied queries.
func NewAccountsAPI(q *sqlc.Queries) *AccountsAPI { return &AccountsAPI{q: q} }

type createAccountReq struct {
	Name     string `json:"name"`
	Currency string `json:"currency"` // e.g., "USD"
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
}

// Create adds an account owned by the authenticated user.
func (a *AccountsAPI) Create(w http.ResponseWriter, r *http.Request) {
	var req createAccountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	row, err := a.q.CreateAccount(r.Context(), sqlc.CreateAccountParams{
		UserID: uid, Name: req.Name, Currency: req.Currency,
	})
	if err != nil {
		http.Error(w, "could not create (duplicate name?)", http.StatusConflict)
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
