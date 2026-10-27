package httptransport

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TransactionsAPI handles ledger writes, category confirmation, and account history.
type TransactionsAPI struct {
	q   *sqlc.Queries
	svc *service.TransactionService
}

// NewTransactionsAPI creates ledger handlers using the supplied service and queries.
func NewTransactionsAPI(q *sqlc.Queries, svc *service.TransactionService) *TransactionsAPI {
	return &TransactionsAPI{q: q, svc: svc}
}

// Routes registers transaction, transfer, category-confirmation, and history endpoints.
func (a *TransactionsAPI) Routes(r chi.Router) {
	r.Post("/transactions", a.CreateTransaction)
	r.Post("/transactions/{id}/reverse", a.ReverseTransaction)
	r.Post("/transactions/{id}/category", a.ConfirmCategory)
	r.Post("/transfers", a.CreateTransfer)
	r.Get("/accounts/{id}/transactions", a.ListAccountEntries)
}

// ConfirmCategory promotes a category onto a transaction — the human side of
// the suggest-then-confirm boundary. Accepts {"category_id": N}; the id must
// be one of the caller's own categories.
func (a *TransactionsAPI) ConfirmCategory(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	txID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || txID <= 0 {
		http.Error(w, "invalid transaction id", http.StatusBadRequest)
		return
	}
	var body struct {
		CategoryID int64 `json:"category_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.CategoryID <= 0 {
		http.Error(w, "category_id required", http.StatusUnprocessableEntity)
		return
	}
	// Ownership of the category (the tx ownership is enforced by the query).
	if _, err := a.q.GetCategoryForUser(r.Context(), sqlc.GetCategoryForUserParams{ID: body.CategoryID, UserID: uid}); err != nil {
		http.Error(w, "unknown category", http.StatusUnprocessableEntity)
		return
	}
	row, err := a.q.ConfirmTransactionCategory(r.Context(), sqlc.ConfirmTransactionCategoryParams{
		ID: txID, UserID: uid, CategoryID: pgtype.Int8{Int64: body.CategoryID, Valid: true},
	})
	if err != nil {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transaction_id": row.ID, "category_id": body.CategoryID})
}

// ReverseTransaction records a linked reversal for a transaction owned by the caller.
func (a *TransactionsAPI) ReverseTransaction(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	txID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || txID <= 0 {
		http.Error(w, "invalid transaction id", http.StatusBadRequest)
		return
	}
	dto, err := a.svc.Reverse(r.Context(), service.ReverseInput{
		UserID:         uid,
		TransactionID:  txID,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto)
}

// CreateTransaction records income or expense on an account owned by the caller.
func (a *TransactionsAPI) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var body struct {
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
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if body.AccountID == 0 || body.Currency == "" || body.OccurredAt.IsZero() {
		http.Error(w, "account_id, currency and occurred_at are required", http.StatusUnprocessableEntity)
		return
	}

	dto, err := a.svc.Create(r.Context(), service.CreateInput{
		UserID:         uid,
		AccountID:      body.AccountID,
		AmountMinor:    body.Amount,
		Currency:       body.Currency,
		OccurredAt:     body.OccurredAt,
		Note:           body.Note,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto)
}

// CreateTransfer moves money between two accounts owned by the caller.
func (a *TransactionsAPI) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FromAccountID int64     `json:"from_account_id"`
		ToAccountID   int64     `json:"to_account_id"`
		Amount        int64     `json:"amount_minor"`
		Currency      string    `json:"currency"`
		OccurredAt    time.Time `json:"occurred_at"`
		Note          string    `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if body.FromAccountID == 0 || body.ToAccountID == 0 || body.Currency == "" || body.OccurredAt.IsZero() {
		http.Error(w, "from_account_id, to_account_id, currency and occurred_at are required", http.StatusUnprocessableEntity)
		return
	}

	dto, err := a.svc.Transfer(r.Context(), service.TransferInput{
		UserID:         uid,
		FromAccountID:  body.FromAccountID,
		ToAccountID:    body.ToAccountID,
		AmountMinor:    body.Amount,
		Currency:       body.Currency,
		OccurredAt:     body.OccurredAt,
		Note:           body.Note,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dto)
}

type entryDTO struct {
	PostingID           int64     `json:"posting_id"`
	TransactionID       int64     `json:"transaction_id"`
	AccountID           int64     `json:"account_id"`
	AmountMinor         int64     `json:"amount_minor"`
	Currency            string    `json:"currency"`
	OccurredAt          time.Time `json:"occurred_at"`
	Note                string    `json:"note,omitempty"`
	CategoryID          *int64    `json:"category_id,omitempty"`           // confirmed
	SuggestedCategoryID *int64    `json:"suggested_category_id,omitempty"` // AI proposal, unconfirmed
	SuggestionScore     *float64  `json:"suggestion_score,omitempty"`
}

// ListAccountEntries returns cursor-paginated postings for an account owned by the caller.
func (a *TransactionsAPI) ListAccountEntries(w http.ResponseWriter, r *http.Request) {
	uid, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || uid == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	accID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || accID <= 0 {
		http.Error(w, "invalid account id", http.StatusBadRequest)
		return
	}
	if _, err := ownedAccount(r.Context(), a.q, uid, accID); err != nil {
		writeServiceError(w, err)
		return
	}

	limit := parseInt(r, "limit", 20)
	if limit > 100 {
		limit = 100
	}
	cursorT, cursorID, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		http.Error(w, "invalid cursor", http.StatusBadRequest)
		return
	}

	rows, err := a.q.ListAccountEntries(r.Context(), sqlc.ListAccountEntriesParams{
		AccountID:        accID,
		CursorOccurredAt: cursorT,
		CursorPostingID:  cursorID,
		PageLimit:        int32(limit),
	})
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	entries := make([]entryDTO, 0, len(rows))
	for _, row := range rows {
		e := entryDTO{
			PostingID:     row.PostingID,
			TransactionID: row.TransactionID,
			AccountID:     row.AccountID,
			AmountMinor:   row.AmountMinor,
			Currency:      row.Currency,
			OccurredAt:    row.OccurredAt,
		}
		if row.Note.Valid {
			e.Note = row.Note.String
		}
		if row.CategoryID.Valid {
			e.CategoryID = &row.CategoryID.Int64
		}
		if row.SuggestedCategoryID.Valid {
			e.SuggestedCategoryID = &row.SuggestedCategoryID.Int64
		}
		if row.SuggestionScore.Valid {
			e.SuggestionScore = &row.SuggestionScore.Float64
		}
		entries = append(entries, e)
	}

	next := ""
	if len(rows) == limit {
		last := rows[len(rows)-1]
		next = encodeCursor(last.OccurredAt, last.PostingID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "next_cursor": next})
}

// Cursor is base64("<RFC3339Nano occurred_at>|<posting id>"); an empty cursor
// means the first (newest) page.
func encodeCursor(t time.Time, id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s|%d", t.UTC().Format(time.RFC3339Nano), id)))
}

func decodeCursor(s string) (time.Time, int64, error) {
	if s == "" {
		// Sentinel far in the future: every row sorts before it.
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), math.MaxInt64, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, 0, err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, 0, errors.New("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, 0, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	return t, id, nil
}

// writeServiceError maps service sentinel errors onto HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrAccountNotFound),
		errors.Is(err, service.ErrTxNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, service.ErrAlreadyReversed),
		errors.Is(err, service.ErrIsReversal):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, service.ErrForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, service.ErrAccountInactive),
		errors.Is(err, service.ErrCurrencyMismatch),
		errors.Is(err, service.ErrUnbalanced),
		errors.Is(err, service.ErrZeroAmount):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, service.ErrIdempotencyConflict):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, service.ErrIdempotencyInFlight):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
