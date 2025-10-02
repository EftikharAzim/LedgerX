package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

type UsersAPI struct {
	q *sqlc.Queries
}

func NewUsersAPI(q *sqlc.Queries) *UsersAPI { return &UsersAPI{q: q} }

type createUserReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type userDTO struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

func (u *UsersAPI) Routes(r chi.Router) {
	r.Post("/users", u.CreateUser)
	r.Get("/users", u.ListUsers)
}

func (u *UsersAPI) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Email == "" || len(req.Password) < 6 {
		http.Error(w, "email and 6+ char password required", http.StatusUnprocessableEntity)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "hash error", http.StatusInternalServerError)
		return
	}

	row, err := u.q.CreateUser(r.Context(), sqlc.CreateUserParams{
		Email:        req.Email,
		PasswordHash: string(hash),
	})
	if err != nil {
		http.Error(w, "could not create (maybe email exists)", http.StatusConflict)
		return
	}

	writeJSON(w, http.StatusCreated, userDTO{ID: row.ID, Email: row.Email})
}

func (u *UsersAPI) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit := parseInt(r, "limit", 20)
	offset := parseInt(r, "offset", 0)
	rows, err := u.q.ListUsers(r.Context(), sqlc.ListUsersParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	out := make([]userDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, userDTO{ID: row.ID, Email: row.Email})
	}
	writeJSON(w, http.StatusOK, out)
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

// small wrapper to allow context timeouts in sqlc calls if needed
func withCtx(r *http.Request) context.Context { return r.Context() }

// demonstrate error mapping pattern (kept minimal here)
func toHTTP(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusInternalServerError
}
