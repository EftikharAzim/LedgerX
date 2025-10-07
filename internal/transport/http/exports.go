package httptransport

import (
    "net/http"
    "path/filepath"
    "strconv"
    "time"

    sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
    "github.com/EftikharAzim/ledgerx/internal/worker"
    "github.com/go-chi/chi/v5"
    "github.com/hibiken/asynq"
)

type ExportsAPI struct {
	q      *sqlc.Queries
	client *asynq.Client
}

func NewExportsAPI(q *sqlc.Queries, redisAddr string) *ExportsAPI {
	return &ExportsAPI{
		q:      q,
		client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}),
	}
}

// PrivateRoutes registers authenticated routes only.
func (e *ExportsAPI) PrivateRoutes(r chi.Router) {
    r.Post("/exports", e.CreateExport)
}

// PublicRoutes registers unauthenticated routes.
func (e *ExportsAPI) PublicRoutes(r chi.Router) {
    r.Get("/exports/{id}/status", e.GetStatus)
    r.Get("/exports/{id}/download", e.Download)
}

func (e *ExportsAPI) CreateExport(w http.ResponseWriter, r *http.Request) {
    monthStr := r.URL.Query().Get("month")
    if monthStr == "" {
        http.Error(w, "missing month", 400)
        return
    }
    month, _ := time.Parse("2006-01", monthStr)

    // Require authenticated user from context (protected route under /v1)
    uid, ok := r.Context().Value(UserIDKey).(int64)
    if !ok || uid == 0 {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    exp, err := e.q.CreateExport(r.Context(), sqlc.CreateExportParams{
        UserID: uid,
        Month:  month,
    })
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
	}

	task := asynq.NewTask(worker.TypeExportCSV, worker.MustJSON(worker.ExportCSVPayload{
		ExportID: exp.ID,
		UserID:   exp.UserID,
		Month:    month.Format("2006-01-02"),
	}))
	_, _ = e.client.Enqueue(task)

    writeJSON(w, http.StatusOK, exp)
}

func (e *ExportsAPI) GetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	exp, err := e.q.GetExportByID(r.Context(), toInt64(id))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
    writeJSON(w, http.StatusOK, exp)
}

func (e *ExportsAPI) Download(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    exp, err := e.q.GetExportByID(r.Context(), toInt64(id))
    if err != nil {
        http.Error(w, err.Error(), 404)
        return
    }
    if !exp.FilePath.Valid || exp.Status != "done" {
        http.Error(w, "not ready", 400)
        return
    }
    w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(exp.FilePath.String))
    http.ServeFile(w, r, exp.FilePath.String)

}

func toInt64(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}
