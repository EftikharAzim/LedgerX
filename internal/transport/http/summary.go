package httptransport

import (
	"net/http"
	"strconv"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
)

type SummaryAPI struct {
	q   *sqlc.Queries
	svc *service.SummaryService
}

func NewSummaryAPI(q *sqlc.Queries, svc *service.SummaryService) *SummaryAPI {
	return &SummaryAPI{q: q, svc: svc}
}

func (a *SummaryAPI) Routes(r chi.Router) {
	r.Get("/accounts/{id}/summary", a.GetSummary)
}

func (a *SummaryAPI) GetSummary(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid account id", http.StatusBadRequest)
		return
	}
	monthStr := r.URL.Query().Get("month")
	month, err := time.Parse("2006-01", monthStr)
	if err != nil {
		http.Error(w, "month must be YYYY-MM", http.StatusBadRequest)
		return
	}
	if _, err := ownedAccount(r.Context(), a.q, uid, id); err != nil {
		writeServiceError(w, err)
		return
	}

	s, err := a.svc.GetMonthlySummary(r.Context(), id, month)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s)
}
