package httptransport

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
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

func (e *ExportsAPI) Routes(r chi.Router) {
	r.Post("/exports", e.CreateExport)
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

	// Determine user from Authorization header if present, otherwise fallback to 1
	userID := int64(1)
	h := r.Header.Get("Authorization")
	if h != "" && strings.HasPrefix(h, "Bearer ") {
		token := strings.TrimPrefix(h, "Bearer ")
		if uid, err := service.ParseJWT(token); err == nil && uid != 0 {
			userID = uid
		}
	}

	exp, err := e.q.CreateExport(r.Context(), sqlc.CreateExportParams{
		UserID: userID,
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

	_ = json.NewEncoder(w).Encode(exp)
}

func (e *ExportsAPI) GetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	exp, err := e.q.GetExportByID(r.Context(), toInt64(id))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	_ = json.NewEncoder(w).Encode(exp)
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

	http.ServeFile(w, r, exp.FilePath.String)

}

func toInt64(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}
