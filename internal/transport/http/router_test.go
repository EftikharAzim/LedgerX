package httptransport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterIgnoresUntrustedForwardingHeaders(t *testing.T) {
	router := NewRouter()
	var peer string
	router.Get("/peer", func(w http.ResponseWriter, r *http.Request) {
		peer = r.RemoteAddr
		w.WriteHeader(http.StatusNoContent)
	})
	for _, header := range []string{"True-Client-IP", "X-Real-IP", "X-Forwarded-For"} {
		t.Run(header, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/peer", nil)
			req.RemoteAddr = "192.0.2.1:12345"
			req.Header.Set(header, "198.51.100.1")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusNoContent || peer != "192.0.2.1:12345" {
				t.Fatalf("untrusted header changed connection peer: %d %q", rec.Code, peer)
			}
		})
	}
}
