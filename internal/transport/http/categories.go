package httptransport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/EftikharAzim/ledgerx/internal/ai"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/go-chi/chi/v5"
	"golang.org/x/text/currency"
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
	r.Post("/categories/suggest", a.Suggest)
}

// Suggest returns a read-only, free history/keyword hint for the caller's draft.
// It deliberately does not use the optional external-model worker backend.
func (a *CategoriesAPI) Suggest(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(UserIDKey).(int64)
	if uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Note     string `json:"note"`
		Currency string `json:"currency"`
		Kind     string `json:"kind"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		http.Error(w, "invalid suggestion request", http.StatusBadRequest)
		return
	}
	body.Note = strings.TrimSpace(body.Note)
	_, currencyErr := currency.ParseISO(body.Currency)
	if len([]rune(body.Note)) < 3 || len([]rune(body.Note)) > 240 || len(body.Currency) != 3 || body.Currency != strings.ToUpper(body.Currency) || currencyErr != nil ||
		(body.Kind != "expense" && body.Kind != "income" && body.Kind != "refund") {
		http.Error(w, "provide a note of 3–240 characters, a currency, and expense, income, or refund kind", http.StatusBadRequest)
		return
	}
	cats, err := a.q.ListCategoriesByUser(r.Context(), uid)
	if err != nil {
		http.Error(w, "could not load categories", http.StatusInternalServerError)
		return
	}
	history, err := a.q.ListConfirmedCategoryMatches(r.Context(), sqlc.ListConfirmedCategoryMatchesParams{UserID: uid, Currency: body.Currency, Kind: body.Kind, Note: body.Note})
	if err != nil {
		http.Error(w, "could not check previous categories", http.StatusInternalServerError)
		return
	}
	candidates := make([]ai.Category, 0, len(cats))
	for _, c := range cats {
		candidates = append(candidates, ai.Category{ID: c.ID, Name: c.Name})
	}
	hint := ai.RecommendPersonal(body.Note, candidates, history)
	name, reason := "", "No clear match yet. Choose a category yourself."
	for _, c := range cats {
		if c.ID == hint.CategoryID {
			name = c.Name
			break
		}
	}
	switch hint.Source {
	case "history":
		if hint.Samples == 1 {
			reason = "You previously chose this category for the same note."
		} else {
			reason = fmt.Sprintf("Used for %d of your last %d matching entries.", hint.Supporting, hint.Samples)
		}
	case "keyword":
		reason = "This category name matches a word in your note."
	case "ambiguous":
		reason = "There is more than one possible category. Choose the one that fits."
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"category_id": hint.CategoryID, "category_name": name, "source": hint.Source, "reason": reason, "supporting_records": hint.Supporting, "matching_records": hint.Samples})
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
