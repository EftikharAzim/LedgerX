package httptransport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
)

type BalanceAPI struct{ svc *service.BalanceService }

func NewBalanceAPI(s *service.BalanceService) *BalanceAPI { return &BalanceAPI{svc: s} }

func (b *BalanceAPI) Routes(r chi.Router) {
	r.Get("/accounts/{id}/balance", b.GetCurrent)
}

func (b *BalanceAPI) GetCurrent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	accID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || accID <= 0 {
		http.Error(w, "invalid account id", http.StatusBadRequest)
		return
	}
	res, err := b.svc.CurrentBalance(r.Context(), accID, time.Now())
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
