package httptransport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type authDatabase interface {
	sqlc.DBTX
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

// AuthAPI handles credentials and rotating refresh-token sessions.
type AuthAPI struct {
	db         authDatabase
	q          *sqlc.Queries
	refreshTTL time.Duration
}

// NewAuthAPI creates auth handlers with the given refresh-token lifetime.
func NewAuthAPI(db authDatabase, refreshTTL time.Duration) *AuthAPI {
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	return &AuthAPI{db: db, q: sqlc.New(db), refreshTTL: refreshTTL}
}

// All token writes take the same user-row lock. READ COMMITTED ensures the
// statements after a lock wait see the preceding holder's committed tokens.
func (a *AuthAPI) beginAuthTx(ctx context.Context, userID int64) (pgx.Tx, *sqlc.Queries, error) {
	tx, err := a.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, nil, err
	}
	qtx := a.q.WithTx(tx)
	if _, err := qtx.LockAuthUser(ctx, userID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}
	return tx, qtx, nil
}

// Routes registers registration, login, refresh, and logout endpoints.
func (a *AuthAPI) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				next.ServeHTTP(w, r)
			})
		})
		r.Post("/auth/register", a.Register)
		r.Post("/auth/login", a.Login)
		r.Post("/auth/refresh", a.Refresh)
		r.Post("/auth/logout", a.Logout)
		r.Post("/auth/recover", a.Recover)
	})
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
	if len(c.Password) < 8 || len(c.Password) > 72 {
		return "password must be between 8 and 72 bytes"
	}
	return ""
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // access-token lifetime, seconds
}

// mintTokens prepares a token pair without changing stored refresh tokens.
func mintTokens(userID, version int64) (tokenResponse, string, error) {
	access, err := service.GenerateSessionJWT(userID, version)
	if err != nil {
		return tokenResponse{}, "", err
	}
	raw, hash, err := service.NewRefreshToken()
	if err != nil {
		return tokenResponse{}, "", err
	}
	return tokenResponse{
		AccessToken:  access,
		RefreshToken: raw,
		TokenType:    "Bearer",
		ExpiresIn:    int(service.AccessTTL().Seconds()),
	}, hash, nil
}

// issueTokens mints an access JWT + a stored refresh token and writes the
// standard token response. Register and login start new sessions here;
// refresh stores its successor atomically with consumption of the old token.
func (a *AuthAPI) issueTokens(w http.ResponseWriter, r *http.Request, userID int64, code int, checkedPasswordHash string) {
	tx, qtx, err := a.beginAuthTx(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	state, err := qtx.GetSecurityState(r.Context(), userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// A password reset can commit between Login's bcrypt check and this lock.
	if state.PasswordHash != checkedPasswordHash {
		http.Error(w, "credentials changed; please log in again", http.StatusUnauthorized)
		return
	}
	tokens, hash, err := mintTokens(userID, state.AuthVersion)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if _, err := qtx.InsertRefreshToken(r.Context(), sqlc.InsertRefreshTokenParams{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(a.refreshTTL),
	}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, code, tokens)
}

// Register creates a user and issues an access token and refresh token.
func (a *AuthAPI) Register(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := readJSON(w, r, 8192, &body); err != nil {
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
	u, err := a.q.CreateUser(r.Context(), sqlc.CreateUserParams{Email: body.Email, PasswordHash: hash})
	if err != nil {
		http.Error(w, "email already registered", http.StatusConflict)
		return
	}
	a.issueTokens(w, r, u.ID, http.StatusCreated, u.PasswordHash)
}

// Login checks credentials and issues an access token and refresh token.
func (a *AuthAPI) Login(w http.ResponseWriter, r *http.Request) {
	var body credentials
	if err := readJSON(w, r, 8192, &body); err != nil {
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
	a.issueTokens(w, r, u.ID, http.StatusOK, u.PasswordHash)
}

// Refresh implements rotating refresh tokens with reuse detection:
//   - unknown / expired token        -> 401
//   - already-revoked token presented -> token theft: revoke ALL of the
//     user's tokens and 401 (an attacker and the user can't both keep a session)
//   - valid token -> revoke it (rotation) and issue a fresh access+refresh pair
func (a *AuthAPI) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := readJSON(w, r, 8192, &body); err != nil || body.RefreshToken == "" {
		http.Error(w, "refresh_token required", http.StatusBadRequest)
		return
	}
	rec, err := a.q.GetRefreshToken(r.Context(), service.HashToken(body.RefreshToken))
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tx, qtx, err := a.beginAuthTx(r.Context(), rec.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	// The first read only identifies the owner. Make all security decisions
	// from a fresh read after taking the lock shared by rotation/revocation.
	rec, err = qtx.GetRefreshToken(r.Context(), rec.TokenHash)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if rec.RevokedAt.Valid {
		// Reuse of a rotated/revoked token — treat as compromise.
		a.rejectRefreshReuse(w, r, tx, qtx, rec.UserID)
		return
	}
	if time.Now().After(rec.ExpiresAt) {
		http.Error(w, "refresh token expired", http.StatusUnauthorized)
		return
	}
	version, err := qtx.GetAuthVersion(r.Context(), rec.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tokens, hash, err := mintTokens(rec.UserID, version)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// The user lock also prevents a descendant rotation from inserting a new
	// token outside a concurrent reuse-revocation statement's snapshot.
	if _, err := qtx.RotateRefreshToken(r.Context(), sqlc.RotateRefreshTokenParams{
		OldTokenHash: rec.TokenHash,
		NewTokenHash: hash,
		ExpiresAt:    time.Now().Add(a.refreshTTL),
	}); errors.Is(err, pgx.ErrNoRows) {
		a.rejectRefreshReuse(w, r, tx, qtx, rec.UserID)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (a *AuthAPI) rejectRefreshReuse(w http.ResponseWriter, r *http.Request, tx pgx.Tx, qtx *sqlc.Queries, userID int64) {
	if err := qtx.RevokeAllUserRefreshTokens(r.Context(), userID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Error(w, "refresh token reuse detected; please log in again", http.StatusUnauthorized)
}

// Logout revokes the presented refresh token. Idempotent: revoking an unknown
// or already-revoked token still returns 204 (don't leak token validity).
func (a *AuthAPI) Logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := readJSON(w, r, 8192, &body); err != nil || body.RefreshToken == "" {
		http.Error(w, "refresh_token required", http.StatusBadRequest)
		return
	}
	rec, err := a.q.GetRefreshToken(r.Context(), service.HashToken(body.RefreshToken))
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tx, qtx, err := a.beginAuthTx(r.Context(), rec.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if _, err := qtx.RevokeRefreshToken(r.Context(), rec.TokenHash); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
