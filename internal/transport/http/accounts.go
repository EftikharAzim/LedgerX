package httptransport

import (
	"encoding/json"
	"net/http"
	"strconv"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/go-chi/chi/v5"
)

type AccountsAPI struct{ q *sqlc.Queries }

func NewAccountsAPI(q *sqlc.Queries) *AccountsAPI { return &AccountsAPI{q: q} }

type createAccountReq struct {
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Currency string `json:"currency"` // e.g., "USD"
}
type accountDTO struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Active   bool   `json:"active"`
}

func (a *AccountsAPI) Routes(r chi.Router) {
	r.Post("/accounts", a.Create)
	r.Get("/accounts", a.List)
}

func (a *AccountsAPI) Create(w http.ResponseWriter, r *http.Request) {
	var req createAccountReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.UserID == 0 || req.Name == "" {
		http.Error(w, "user_id and name required", http.StatusUnprocessableEntity)
		return
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}

	row, err := a.q.CreateAccount(r.Context(), sqlc.CreateAccountParams{
		UserID: req.UserID, Name: req.Name, Currency: req.Currency,
	})
	if err != nil {
		http.Error(w, "could not create (duplicate name?)", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusCreated, accountDTO{
		ID: row.ID, UserID: row.UserID, Name: row.Name, Currency: row.Currency, Active: row.IsActive,
	})
}

func (a *AccountsAPI) List(w http.ResponseWriter, r *http.Request) {
	userID := parseInt64(r, "user_id", 0)
	if userID == 0 {
		http.Error(w, "user_id required", http.StatusBadRequest)
		return
	}
	limit := parseInt(r, "limit", 20)
	offset := parseInt(r, "offset", 0)

	rows, err := a.q.ListAccountsByUser(r.Context(), sqlc.ListAccountsByUserParams{
		UserID: userID,
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
			ID: row.ID, UserID: row.UserID, Name: row.Name, Currency: row.Currency, Active: row.IsActive,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func parseInt64(r *http.Request, key string, def int64) int64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return def
	}
	return n
}
