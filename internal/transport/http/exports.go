package httptransport

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/reporting"
	"github.com/EftikharAzim/ledgerx/internal/storage"
	"github.com/EftikharAzim/ledgerx/internal/worker"
	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// ExportsAPI handles CSV report requests and owner-only export reads.
type ExportsAPI struct {
	q      *sqlc.Queries
	client *asynq.Client
	store  storage.Storage
}

type exportDTO struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	Month      string    `json:"month,omitempty"`
	ReportType string    `json:"report_type"`
	StartDate  string    `json:"start_date,omitempty"`
	EndDate    string    `json:"end_date,omitempty"`
	Status     string    `json:"status"`
	FilePath   string    `json:"file_path,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func toExportDTO(e sqlc.Export) exportDTO {
	dto := exportDTO{
		ID:         e.ID,
		UserID:     e.UserID,
		Status:     e.Status,
		ReportType: e.ReportType,
	}
	if e.Month.Valid {
		dto.Month = e.Month.Time.Format("2006-01")
	}
	if e.StartDate.Valid {
		dto.StartDate = e.StartDate.Time.Format("2006-01-02")
	}
	if e.EndDate.Valid {
		dto.EndDate = e.EndDate.Time.Format("2006-01-02")
	}
	if e.FilePath.Valid {
		dto.FilePath = e.FilePath.String
	}
	if e.CreatedAt.Valid {
		dto.CreatedAt = e.CreatedAt.Time
	}
	if e.UpdatedAt.Valid {
		dto.UpdatedAt = e.UpdatedAt.Time
	}
	return dto
}

// NewExportsAPI creates export handlers with a Redis task client and file storage.
func NewExportsAPI(q *sqlc.Queries, redisAddr string, store storage.Storage) *ExportsAPI {
	return &ExportsAPI{
		q:      q,
		client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}),
		store:  store,
	}
}

// Routes registers all export endpoints; every one requires auth and
// checks that the export belongs to the caller.
func (e *ExportsAPI) Routes(r chi.Router) {
	r.Post("/exports", e.CreateExport)
	r.Get("/exports/{id}/status", e.GetStatus)
	r.Get("/exports/{id}/download", e.Download)
}

// CreateExport records and enqueues a validated report period for the caller.
func (e *ExportsAPI) CreateExport(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	values := r.URL.Query()
	period, err := reporting.Parse(reporting.Selection{Type: values.Get("type"), Month: values.Get("month"), WeekOf: values.Get("week_of"), Year: values.Get("year"), StartDate: values.Get("start_date"), EndDate: values.Get("end_date")})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	exp, err := e.q.CreateExport(r.Context(), sqlc.CreateExportParams{
		UserID:     uid,
		Month:      pgtype.Date{Time: period.Start, Valid: period.Month != ""},
		ReportType: period.Type,
		StartDate:  pgtype.Date{Time: period.Start, Valid: period.Type != "all"},
		EndDate:    pgtype.Date{Time: period.End, Valid: period.Type != "all"},
	})
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	task := asynq.NewTask(worker.TypeExportCSV, worker.MustJSON(worker.ExportCSVPayload{
		ExportID: exp.ID,
		UserID:   exp.UserID,
	}))
	if _, err := e.client.EnqueueContext(r.Context(), task); err != nil {
		// Do not leave a request looking queued if Redis rejected the job.
		statusCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
		defer cancel()
		_ = e.q.UpdateExportStatus(statusCtx, sqlc.UpdateExportStatusParams{ID: exp.ID, Status: "error"})
		http.Error(w, "enqueue failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, toExportDTO(exp))
}

// ownedExport loads an export and verifies the caller owns it.
func (e *ExportsAPI) ownedExport(w http.ResponseWriter, r *http.Request) (sqlc.Export, bool) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return sqlc.Export{}, false
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid export id", http.StatusBadRequest)
		return sqlc.Export{}, false
	}
	exp, err := e.q.GetExportByID(r.Context(), id)
	if err != nil || exp.UserID != uid {
		http.Error(w, "not found", http.StatusNotFound)
		return sqlc.Export{}, false
	}
	return exp, true
}

// GetStatus returns export metadata after checking ownership.
func (e *ExportsAPI) GetStatus(w http.ResponseWriter, r *http.Request) {
	exp, ok := e.ownedExport(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toExportDTO(exp))
}

// Download streams a completed export after checking ownership.
func (e *ExportsAPI) Download(w http.ResponseWriter, r *http.Request) {
	exp, ok := e.ownedExport(w, r)
	if !ok {
		return
	}
	if !exp.FilePath.Valid || exp.Status != "done" {
		http.Error(w, "not ready", http.StatusConflict)
		return
	}
	body, err := e.store.Open(r.Context(), exp.FilePath.String)
	if err != nil {
		http.Error(w, "export file unavailable", http.StatusInternalServerError)
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+exp.FilePath.String)
	_, _ = io.Copy(w, body)
}
