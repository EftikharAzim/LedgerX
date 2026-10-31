package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	httptransport "github.com/EftikharAzim/ledgerx/internal/transport/http"
	"github.com/go-chi/chi/v5"
)

func TestHistoryExposesTransferAndReversalRelationships(t *testing.T) {
	ctx := context.Background()
	uid, source := newUserWithAccount(t, "USD")
	target, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{UserID: uid, Name: "History target", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	income, err := txSvc.Create(ctx, service.CreateInput{UserID: uid, AccountID: source, AmountMinor: 10000, Currency: "USD", OccurredAt: time.Now().UTC(), Note: "reversal of 123 is just a note"})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := txSvc.Transfer(ctx, service.TransferInput{UserID: uid, FromAccountID: source, ToAccountID: target.ID, AmountMinor: 1200, Currency: "USD", OccurredAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	reversal, err := txSvc.Reverse(ctx, service.ReverseInput{UserID: uid, TransactionID: transfer.ID})
	if err != nil {
		t.Fatal(err)
	}
	external, err := q.GetExternalAccount(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), httptransport.UserIDKey, uid)))
		})
	})
	httptransport.NewTransactionsAPI(q, txSvc).Routes(router)
	for _, accountID := range []int64{source, target.ID, external.ID} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/accounts/%d/transactions", accountID), nil))
		if response.Code != http.StatusOK {
			t.Fatalf("history status %d: %s", response.Code, response.Body.String())
		}
		var body struct {
			Entries []struct {
				TransactionID int64  `json:"transaction_id"`
				IsTransfer    bool   `json:"is_transfer"`
				ReversalOf    *int64 `json:"reversal_of"`
				ReversedBy    *int64 `json:"reversed_by"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		seen := make(map[int64]bool)
		for _, entry := range body.Entries {
			seen[entry.TransactionID] = true
			switch entry.TransactionID {
			case income.ID:
				if entry.IsTransfer || entry.ReversalOf != nil || entry.ReversedBy != nil {
					t.Fatalf("income note must not imply a reversal: %+v", entry)
				}
			case transfer.ID:
				if !entry.IsTransfer || entry.ReversedBy == nil || *entry.ReversedBy != reversal.ID || entry.ReversalOf != nil {
					t.Fatalf("missing original relationship: %+v", entry)
				}
			case reversal.ID:
				if !entry.IsTransfer || entry.ReversalOf == nil || *entry.ReversalOf != transfer.ID || entry.ReversedBy != nil {
					t.Fatalf("missing reversal relationship: %+v", entry)
				}
			}
		}
		if accountID == external.ID {
			if !seen[income.ID] || seen[transfer.ID] || seen[reversal.ID] {
				t.Fatalf("external account should only see the income: %+v", seen)
			}
		} else if !seen[transfer.ID] || !seen[reversal.ID] || (accountID == source && !seen[income.ID]) {
			t.Fatalf("history omitted expected transactions: %+v", seen)
		}
	}
}
