package httptransport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestJSONConsumesOneBoundedDocument(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		limit      int64
		strict     bool
		valid      bool
	}{
		{"valid", `{"name":"rent"}`, 64, false, true},
		{"trailing whitespace", "{\"name\":\"rent\"}\n\t ", 64, false, true},
		{"compatible unknown field", `{"name":"rent","extra":1}`, 64, false, true},
		{"strict unknown field", `{"name":"rent","extra":1}`, 64, true, false},
		{"empty", "", 64, false, false},
		{"malformed", `{"name":`, 64, false, false},
		{"second document", `{"name":"rent"} {"name":"changed"}`, 64, false, false},
		{"trailing garbage", `{"name":"rent"} garbage`, 64, false, false},
		{"body too large", `{"name":"rent"}`, 8, false, false},
		{"exact byte limit", `{"name":"rent"}`, 15, false, true},
		{"oversized trailing whitespace", `{"name":"rent"}` + strings.Repeat(" ", 100), 16, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var input struct {
				Name string `json:"name"`
			}
			err := readRequestJSON(httptest.NewRecorder(), req, tc.limit, &input, tc.strict)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}

func TestMalformedWritesFailBeforeReachingServices(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		body    string
	}{
		{"registration", (&AuthAPI{}).Register, `{"email":"test@example.com","password":"password123"} {}`},
		{"account", (&AccountsAPI{}).Create, `{"name":"Wallet","currency":"USD"} trailing`},
		{"transaction", (&TransactionsAPI{}).CreateTransaction, `{"account_id":1} {}`},
		{"transfer", (&TransactionsAPI{}).CreateTransfer, `{"from_account_id":1} {}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			tc.handler(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid write reached the service: status %d", response.Code)
			}
		})
	}
}
