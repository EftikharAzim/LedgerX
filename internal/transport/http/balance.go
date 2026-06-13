package httptransport

import (
	"net/http"
	"strconv"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
)

type BalanceAPI struct {
	q   *sqlc.Queries
	svc *service.BalanceService
}

func NewBalanceAPI(q *sqlc.Queries, s *service.BalanceService) *BalanceAPI {
	return &BalanceAPI{q: q, svc: s}
}

func (b *BalanceAPI) Routes(r chi.Router) {
	r.Get("/accounts/{id}/balance", b.GetCurrent)
}

func (b *BalanceAPI) GetCurrent(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	idStr := chi.URLParam(r, "id")
	accID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || accID <= 0 {
		http.Error(w, "invalid account id", http.StatusBadRequest)
		return
	}
	if _, err := ownedAccount(r.Context(), b.q, uid, accID); err != nil {
		writeServiceError(w, err)
		return
	}
	res, err := b.svc.CurrentBalance(r.Context(), accID, time.Now())
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
