package httptransport

import (
	"encoding/json"
	"net/http"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type TransactionsAPI struct {
	q          *sqlc.Queries
	summarySvc *service.SummaryService
}

func NewTransactionsAPI(q *sqlc.Queries, summarySvc *service.SummaryService) *TransactionsAPI {
	return &TransactionsAPI{q: q, summarySvc: summarySvc}
}

func (a *TransactionsAPI) Routes(r chi.Router) {
	r.Route("/transactions", func(rt chi.Router) {
		rt.Post("/", a.CreateTransaction)
	})
}

func (a *TransactionsAPI) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID     int64     `json:"user_id"`
		AccountID  int64     `json:"account_id"`
		Amount     int64     `json:"amount_minor"`
		Currency   string    `json:"currency"`
		OccurredAt time.Time `json:"occurred_at"`
		Note       string    `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// 1. Insert transaction
	tx, err := a.q.CreateTransaction(r.Context(), sqlc.CreateTransactionParams{
		UserID:      body.UserID,
		AccountID:   body.AccountID,
		AmountMinor: body.Amount,
		Currency:    body.Currency,
		OccurredAt:  body.OccurredAt,
		Note:        pgtype.Text{String: body.Note, Valid: body.Note != ""},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Prepare event payload
	event := map[string]any{
		"transaction_id": tx.ID,
		"account_id":     tx.AccountID,
		"amount_minor":   tx.AmountMinor,
		"currency":       tx.Currency,
		"occurred_at":    tx.OccurredAt,
	}
	payload, _ := json.Marshal(event)

	// 3. Insert event into outbox
	_, err = a.q.CreateOutboxEvent(r.Context(), sqlc.CreateOutboxEventParams{
		EventType: "TransactionCreated",
		Payload:   payload,
	})
	if err != nil {
		http.Error(w, "failed to enqueue event", http.StatusInternalServerError)
		return
	}

	// 4. Bust summary cache
	month := time.Date(body.OccurredAt.Year(), body.OccurredAt.Month(), 1, 0, 0, 0, 0, time.UTC)
	a.summarySvc.Invalidate(r.Context(), body.AccountID, month)

	// 5. Respond
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tx)
}
