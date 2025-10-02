package httptransport

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
)

type SummaryAPI struct {
	svc *service.SummaryService
}

func NewSummaryAPI(svc *service.SummaryService) *SummaryAPI { return &SummaryAPI{svc: svc} }

func (a *SummaryAPI) Routes(r chi.Router) {
	r.Get("/accounts/{id}/summary", a.GetSummary)
}

func (a *SummaryAPI) GetSummary(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	monthStr := r.URL.Query().Get("month")
	if monthStr == "" {
		http.Error(w, "missing month=YYYY-MM", 400)
		return
	}
	month, _ := time.Parse("2006-01", monthStr)

	s, err := a.svc.GetMonthlySummary(r.Context(), id, month)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(s)
}
