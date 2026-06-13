package httptransport

import (
	"encoding/json"
	"net/http"
	"strings"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
)

type AuthAPI struct {
	q *sqlc.Queries
}

func NewAuthAPI(q *sqlc.Queries) *AuthAPI { return &AuthAPI{q: q} }

func (a *AuthAPI) Routes(r chi.Router) {
	r.Post("/auth/register", a.Register)
	r.Post("/auth/login", a.Login)
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (c *credentials) validate() string {
	c.Email = strings.TrimSpace(strings.ToLower(c.Email))
	if c.Email == "" || !strings.Contains(c.Email, "@") {
		return "valid email required"
	}
	if len(c.Password) < 8 {
		return "password must be at least 8 characters"
	}
	return ""
}

func (a *AuthAPI) Register(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if msg := body.validate(); msg != "" {
		http.Error(w, msg, http.StatusUnprocessableEntity)
		return
	}

	hash, err := service.HashPassword(body.Password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	u, err := a.q.CreateUser(r.Context(), sqlc.CreateUserParams{
		Email:        body.Email,
		PasswordHash: hash,
	})
	if err != nil {
		// Most likely the unique-email constraint; never echo DB internals.
		http.Error(w, "email already registered", http.StatusConflict)
		return
	}

	tok, err := service.GenerateJWT(u.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": tok})
}

func (a *AuthAPI) Login(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))

	u, err := a.q.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := service.CheckPassword(u.PasswordHash, body.Password); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	tok, err := service.GenerateJWT(u.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}
