package httptransport

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/EftikharAzim/ledgerx/internal/workspace"
	"github.com/go-chi/chi/v5"
)

// WorkspaceAPI serves private JSON backup downloads and reviewed restores.
type WorkspaceAPI struct {
	q   *sqlc.Queries
	svc *service.WorkspaceService
}

// NewWorkspaceAPI wires workspace tools to the existing database.
func NewWorkspaceAPI(q *sqlc.Queries, svc *service.WorkspaceService) *WorkspaceAPI {
	return &WorkspaceAPI{q, svc}
}

// Routes keeps recovery tools secondary to everyday tracking.
func (a *WorkspaceAPI) Routes(r chi.Router) {
	r.Get("/workspace", a.State)
	r.Get("/workspace/backup", a.Backup)
	r.Post("/workspace/backup", a.Backup)
	r.Post("/workspace/restores/preview", a.Preview)
	r.Get("/workspace/restores/{id}", a.Get)
	r.Post("/workspace/restores/{id}/commit", a.Restore)
}

// State reports whether the workspace is empty and lists recent private reviews.
func (a *WorkspaceAPI) State(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	empty, err := a.q.WorkspaceIsEmpty(r.Context(), uid)
	if err != nil {
		http.Error(w, "could not read workspace", 500)
		return
	}
	rows, err := a.q.ListRestoreSessions(r.Context(), uid)
	if err != nil {
		http.Error(w, "could not read restore history", 500)
		return
	}
	accounts, err := a.q.ListAccountsByUser(r.Context(), sqlc.ListAccountsByUserParams{UserID: uid, Limit: 1001, Offset: 0})
	if err != nil {
		http.Error(w, "could not read accounts", 500)
		return
	}
	accountViews := []map[string]any{}
	for _, account := range accounts {
		if account.Kind == "normal" {
			accountViews = append(accountViews, map[string]any{"id": account.ID, "name": account.Name, "currency": account.Currency, "active": account.IsActive, "kind": account.Kind})
		}
	}
	sessions := []map[string]any{}
	for _, row := range rows {
		sessions = append(sessions, map[string]any{"id": row.ID, "filename": row.Filename, "status": row.Status, "summary": json.RawMessage(row.Summary), "receipt": json.RawMessage(row.Result), "created_at": row.CreatedAt, "expires_at": row.ExpiresAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"empty": empty.Bool, "accounts": accountViews, "restores": sessions})
}

// Backup downloads authoritative financial data, without credentials or jobs.
func (a *WorkspaceAPI) Backup(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Appearance map[int64]workspace.Appearance `json:"appearance"`
	}
	if r.Method == http.MethodPost {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || len(body.Appearance) > 1000 {
			http.Error(w, "invalid appearance preferences", 400)
			return
		}
	}
	b, err := a.svc.Backup(r.Context(), uid, body.Appearance)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	raw, err := json.Marshal(b)
	if err != nil {
		http.Error(w, "could not create backup", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="ledgerx-workspace-`+time.Now().UTC().Format("2006-01-02")+`.ledgerx.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// Preview accepts the original backup bytes and validates the complete workspace.
func (a *WorkspaceAPI) Preview(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workspace.MaxBytes))
	if err != nil {
		http.Error(w, "backup must be no larger than 32 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	out, err := a.svc.Preview(r.Context(), uid, r.URL.Query().Get("filename"), raw)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// Get resumes an owned review/receipt without returning the financial payload.
func (a *WorkspaceAPI) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	out, err := a.svc.Get(r.Context(), uid, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Restore commits precisely the file whose checksum the customer reviewed.
func (a *WorkspaceAPI) Restore(w http.ResponseWriter, r *http.Request) {
	uid, ok := importUser(w, r)
	if !ok {
		return
	}
	id, ok := importID(w, r)
	if !ok {
		return
	}
	var body struct {
		Checksum string `json:"checksum"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		http.Error(w, "invalid restore confirmation", 400)
		return
	}
	out, err := a.svc.Restore(r.Context(), uid, id, body.Checksum)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
