package httptransport

import (
	"encoding/json"
	"net/http"
	"strings"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/go-chi/chi/v5"
)

// CategoriesAPI handles user-owned transaction categories.
type CategoriesAPI struct{ q *sqlc.Queries }

// NewCategoriesAPI creates category handlers using the supplied queries.
func NewCategoriesAPI(q *sqlc.Queries) *CategoriesAPI { return &CategoriesAPI{q: q} }

type categoryDTO struct {
	ID     int64  `json:"id"`
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
}

// Routes registers category creation and listing endpoints.
func (a *CategoriesAPI) Routes(r chi.Router) {
	r.Post("/categories", a.Create)
	r.Get("/categories", a.List)
}

// Create adds a category owned by the authenticated user.
func (a *CategoriesAPI) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		http.Error(w, "name required", http.StatusUnprocessableEntity)
		return
	}
	row, err := a.q.CreateCategory(r.Context(), sqlc.CreateCategoryParams{UserID: uid, Name: body.Name})
	if err != nil {
		// Unique (user_id, lower(name)) violation -> duplicate.
		http.Error(w, "could not create (duplicate name?)", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusCreated, categoryDTO{ID: row.ID, UserID: row.UserID, Name: row.Name})
}

// List returns the authenticated user's categories.
func (a *CategoriesAPI) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rows, err := a.q.ListCategoriesByUser(r.Context(), uid)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	out := make([]categoryDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, categoryDTO{ID: row.ID, UserID: row.UserID, Name: row.Name})
	}
	writeJSON(w, http.StatusOK, out)
}
