package httptransport

import (
	"encoding/json"
	"net/http"
	"strconv"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/go-chi/chi/v5"
)

// UsersAPI serves the authenticated user's profile.
type UsersAPI struct {
	q *sqlc.Queries
}

// NewUsersAPI creates user-profile handlers using the supplied queries.
func NewUsersAPI(q *sqlc.Queries) *UsersAPI { return &UsersAPI{q: q} }

type userDTO struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

// Routes registers the current-user profile endpoint.
func (u *UsersAPI) Routes(r chi.Router) {
	r.Get("/users/me", u.Me)
}

// Me returns the authenticated user's ID and email address.
func (u *UsersAPI) Me(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	row, err := u.q.GetUserByID(r.Context(), uid)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, userDTO{ID: row.ID, Email: row.Email})
}

func parseInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
