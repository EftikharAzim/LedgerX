package httptransport

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var errSecurityCredentials = errors.New("invalid credentials or recovery code")

type securityInput struct {
	Email           string `json:"email"`
	CurrentPassword string `json:"current_password"`
	Password        string `json:"password"`
	RecoveryCode    string `json:"recovery_code"`
	Confirmation    string `json:"confirmation"`
}

func readSecurityInput(w http.ResponseWriter, r *http.Request) (securityInput, bool) {
	w.Header().Set("Cache-Control", "no-store")
	var input securityInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid security request", http.StatusBadRequest)
		return input, false
	}
	return input, true
}

// SecurityRoutes registers authenticated account-lifecycle controls.
func (a *AuthAPI) SecurityRoutes(r chi.Router) {
	r.Get("/security", a.SecurityState)
	r.Post("/security/recovery-code", a.RecoveryCode)
	r.Post("/security/password", a.ChangePassword)
	r.Delete("/security/account", a.DeleteAccount)
}

// SecurityState reports whether a recovery code has been configured, never its hash.
func (a *AuthAPI) SecurityState(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(UserIDKey).(int64)
	state, err := a.q.GetSecurityState(r.Context(), uid)
	if err != nil {
		securityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": state.Email, "recovery_enabled": state.RecoveryCodeHash.Valid})
}

// securityTx takes locks in the same order as financial writers. Export workers
// hold a separate shared lease, preventing a report from appearing after erasure.
func (a *AuthAPI) securityTx(r *http.Request, deleting bool) (pgx.Tx, *sqlc.Queries, sqlc.GetSecurityStateRow, error) {
	uid, _ := r.Context().Value(UserIDKey).(int64)
	version, _ := r.Context().Value(authVersionKey).(int64)
	tx, err := a.db.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, nil, sqlc.GetSecurityStateRow{}, err
	}
	q := a.q.WithTx(tx)
	state := sqlc.GetSecurityStateRow{}
	if deleting {
		err = q.LockUserExports(r.Context(), uid)
		if err == nil {
			err = q.LockUserLedger(r.Context(), uid)
		}
	}
	if err == nil {
		_, err = q.LockAuthUser(r.Context(), uid)
	}
	if err == nil {
		state, err = q.GetSecurityState(r.Context(), uid)
	}
	if err == nil && state.AuthVersion != version {
		err = errSecurityCredentials
	}
	if err != nil {
		_ = tx.Rollback(r.Context())
		return nil, nil, state, err
	}
	return tx, q, state, nil
}

func securityError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, errSecurityCredentials) {
		http.Error(w, errSecurityCredentials.Error(), http.StatusUnauthorized)
		return
	}
	http.Error(w, "security operation unavailable; please try again", http.StatusInternalServerError)
}

// RecoveryCode replaces the previous code and reveals the new one exactly once.
func (a *AuthAPI) RecoveryCode(w http.ResponseWriter, r *http.Request) {
	input, ok := readSecurityInput(w, r)
	if !ok {
		return
	}
	tx, q, state, err := a.securityTx(r, false)
	if err != nil {
		securityError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if service.CheckPassword(state.PasswordHash, input.CurrentPassword) != nil {
		http.Error(w, "current password does not match", http.StatusForbidden)
		return
	}
	raw, _, err := service.NewRefreshToken()
	if err != nil {
		securityError(w, err)
		return
	}
	code := "lxr_" + raw
	if err := q.SetRecoveryCode(r.Context(), sqlc.SetRecoveryCodeParams{ID: state.ID, RecoveryCodeHash: pgtype.Text{String: service.HashToken(code), Valid: true}}); err != nil {
		securityError(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		securityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"recovery_code": code})
}

// ChangePassword invalidates all existing access and refresh sessions.
func (a *AuthAPI) ChangePassword(w http.ResponseWriter, r *http.Request) {
	input, ok := readSecurityInput(w, r)
	if !ok {
		return
	}
	if len(input.Password) < 8 || len(input.Password) > 72 {
		http.Error(w, "password must be between 8 and 72 bytes", http.StatusUnprocessableEntity)
		return
	}
	tx, q, state, err := a.securityTx(r, false)
	if err != nil {
		securityError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if service.CheckPassword(state.PasswordHash, input.CurrentPassword) != nil {
		http.Error(w, "current password does not match", http.StatusForbidden)
		return
	}
	hash, err := service.HashPassword(input.Password)
	if err == nil {
		err = q.ChangePassword(r.Context(), sqlc.ChangePasswordParams{ID: state.ID, PasswordHash: hash})
	}
	if err == nil {
		err = q.RevokeAllUserRefreshTokens(r.Context(), state.ID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		securityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Recover consumes a saved high-entropy code and ends all sessions. It requires
// no email delivery service; users must save a code before losing their password.
func (a *AuthAPI) Recover(w http.ResponseWriter, r *http.Request) {
	input, ok := readSecurityInput(w, r)
	if !ok {
		return
	}
	if len(input.Password) < 8 || len(input.Password) > 72 {
		http.Error(w, "password must be between 8 and 72 bytes", http.StatusUnprocessableEntity)
		return
	}
	user, err := a.q.GetUserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(input.Email)))
	if err != nil {
		securityError(w, err)
		return
	}
	tx, q, err := a.beginAuthTx(r.Context(), user.ID)
	if err != nil {
		securityError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	state, err := q.GetSecurityState(r.Context(), user.ID)
	if err != nil {
		securityError(w, err)
		return
	}
	hashCode := service.HashToken(strings.TrimSpace(input.RecoveryCode))
	if !state.RecoveryCodeHash.Valid || subtle.ConstantTimeCompare([]byte(hashCode), []byte(state.RecoveryCodeHash.String)) != 1 {
		securityError(w, errSecurityCredentials)
		return
	}
	hash, err := service.HashPassword(input.Password)
	if err == nil {
		err = q.RecoverPassword(r.Context(), sqlc.RecoverPasswordParams{ID: user.ID, PasswordHash: hash})
	}
	if err == nil {
		err = q.RevokeAllUserRefreshTokens(r.Context(), user.ID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		securityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteAccount erases authoritative live records and queues durable object
// cleanup in the same transaction. Downloaded copies and server backups are separate.
func (a *AuthAPI) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	input, ok := readSecurityInput(w, r)
	if !ok {
		return
	}
	if input.Confirmation != "DELETE" {
		http.Error(w, "type DELETE to confirm account deletion", http.StatusUnprocessableEntity)
		return
	}
	tx, q, state, err := a.securityTx(r, true)
	if err != nil {
		securityError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if service.CheckPassword(state.PasswordHash, input.CurrentPassword) != nil {
		http.Error(w, "current password does not match", http.StatusForbidden)
		return
	}
	err = q.QueueExportDeletion(r.Context(), state.ID)
	if err == nil {
		err = q.DeleteUserOutbox(r.Context(), state.ID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), "INSERT INTO privacy_deletions(user_id) VALUES ($1) ON CONFLICT DO NOTHING", state.ID)
	}
	if err == nil {
		_, err = q.DeleteUser(r.Context(), state.ID)
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		securityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
