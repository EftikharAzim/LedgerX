package httptransport

import (
    "crypto/sha256"
    "encoding/hex"
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
    // Derive user from auth context
    uid, ok := r.Context().Value(UserIDKey).(int64)
    if !ok || uid == 0 {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    // Basic validation
    if body.AccountID == 0 || body.Currency == "" || body.OccurredAt.IsZero() {
        http.Error(w, "missing required fields", http.StatusUnprocessableEntity)
        return
    }

    // Ownership check: ensure the account belongs to the authenticated user
    acc, err := a.q.GetAccount(r.Context(), body.AccountID)
    if err != nil || acc.UserID != uid {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }

    // Optional idempotency: if header provided, attempt to use idempotency registry
    if key := r.Header.Get("Idempotency-Key"); key != "" {
        // hash request payload as canonical json (with derived uid)
        payload := map[string]any{
            "user_id":     uid,
            "account_id":  body.AccountID,
            "amount_minor": body.Amount,
            "currency":    body.Currency,
            "occurred_at": body.OccurredAt,
            "note":        body.Note,
        }
        b, _ := json.Marshal(payload)
        sum := sha256.Sum256(b)
        hash := hex.EncodeToString(sum[:])

        // Upsert/start idempotency tracking
        rec, err := a.q.UpsertIdempotencyStart(r.Context(), sqlc.UpsertIdempotencyStartParams{
            UserID:      uid,
            Key:         key,
            RequestHash: hash,
        })
        if err == nil {
            // Existing success with same hash: return cached response
            if rec.Status == "succeeded" && rec.RequestHash == hash && rec.ResponseJson.Valid {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusOK)
                _, _ = w.Write([]byte(rec.ResponseJson.String))
                return
            }
            // Key reused with different payload
            if rec.RequestHash != hash {
                http.Error(w, "idempotency key reuse with different payload", http.StatusConflict)
                return
            }
        }

        // Proceed to create and then mark success
        tx, err := a.q.CreateTransaction(r.Context(), sqlc.CreateTransactionParams{
            UserID:      uid,
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
        // Cache response JSON
        resp, _ := json.Marshal(tx)
        _ = a.q.MarkIdempotencySuccess(r.Context(), sqlc.MarkIdempotencySuccessParams{
            UserID:      uid,
            Key:         key,
            RequestHash: hash,
            ResponseJson: pgtype.Text{String: string(resp), Valid: true},
        })
        writeJSON(w, http.StatusOK, tx)
        return
    }

	// 1. Insert transaction
    tx, err := a.q.CreateTransaction(r.Context(), sqlc.CreateTransactionParams{
        UserID:      uid,
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
    writeJSON(w, http.StatusOK, tx)
}
