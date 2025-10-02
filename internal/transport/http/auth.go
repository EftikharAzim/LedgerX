package httptransport

import (
	"encoding/json"
	"net/http"

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

func (a *AuthAPI) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	hash, _ := service.HashPassword(body.Password)

	u, err := a.q.CreateUser(r.Context(), sqlc.CreateUserParams{
		Email:        body.Email,
		PasswordHash: hash,
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	tok, _ := service.GenerateJWT(u.ID)
	json.NewEncoder(w).Encode(map[string]string{"token": tok})
}

func (a *AuthAPI) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	u, err := a.q.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		http.Error(w, "invalid credentials", 401)
		return
	}

	if err := service.CheckPassword(u.PasswordHash, body.Password); err != nil {
		http.Error(w, "invalid credentials", 401)
		return
	}

	tok, _ := service.GenerateJWT(u.ID)
	json.NewEncoder(w).Encode(map[string]string{"token": tok})
}
