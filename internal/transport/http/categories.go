package httptransport

import (
	"encoding/json"
	"fmt"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"net/http"
	"strconv"
	"strings"

	"github.com/EftikharAzim/ledgerx/internal/ai"
	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/go-chi/chi/v5"
	"golang.org/x/text/currency"
)

// CategoriesAPI handles user-owned transaction categories.
type CategoriesAPI struct {
	q   *sqlc.Queries
	svc *service.CategoryService
}

// NewCategoriesAPI creates category handlers using the supplied queries.
func NewCategoriesAPI(q *sqlc.Queries, svc *service.CategoryService) *CategoriesAPI {
	return &CategoriesAPI{q: q, svc: svc}
}

type categoryDTO struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	ParentID int64  `json:"parent_id,omitempty"`
	Icon     string `json:"icon"`
	Kind     string `json:"kind"`
	Archived bool   `json:"archived"`
}

// Routes registers category creation and listing endpoints.
func (a *CategoriesAPI) Routes(r chi.Router) {
	r.Post("/categories", a.Create)
	r.Get("/categories", a.List)
	r.Post("/categories/suggest", a.Suggest)
	r.Patch("/categories/{id}", a.Update)
	r.Get("/categories/{id}/impact", a.Impact)
	r.Delete("/categories/{id}", a.Delete)
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
	cats, err := a.q.ListSelectableCategoriesByUser(r.Context(), uid)
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
	var body service.CategoryEdit
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		http.Error(w, "invalid category", 400)
		return
	}
	row, err := a.svc.Save(r.Context(), uid, 0, body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 201, categoryResponse(row))
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
		out = append(out, categoryResponse(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func categoryResponse(c sqlc.Category) categoryDTO {
	return categoryDTO{ID: c.ID, UserID: c.UserID, Name: c.Name, ParentID: c.ParentID.Int64, Icon: c.Icon, Kind: c.Kind, Archived: c.Archived}
}
func categoryID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id
}

// Update changes category organization while preserving recorded money.
func (a *CategoriesAPI) Update(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	var body service.CategoryEdit
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		http.Error(w, "invalid category", 400)
		return
	}
	if categoryID(r) <= 0 {
		http.Error(w, "invalid category id", 400)
		return
	}
	c, err := a.svc.Save(r.Context(), uid, categoryID(r), body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, categoryResponse(c))
}

// Impact previews records affected by category deletion.
func (a *CategoriesAPI) Impact(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	v, err := a.svc.Impact(r.Context(), uid, categoryID(r))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, v)
}

// Delete requires a fresh usage review and explicit reassignment destination.
func (a *CategoriesAPI) Delete(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	var body struct {
		ReplacementID int64  `json:"replacement_id"`
		Token         string `json:"review_token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		http.Error(w, "invalid reassignment", 400)
		return
	}
	if err := a.svc.Delete(r.Context(), uid, categoryID(r), body.ReplacementID, body.Token); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"deleted": true})
}
