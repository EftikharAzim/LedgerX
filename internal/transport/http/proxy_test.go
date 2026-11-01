package httptransport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedIngressPreservesDistinctClientsAndRejectsSpoofing(t *testing.T) {
	middleware, err := TrustedProxyMiddleware("10.20.0.0/24,2001:db8:1::/64")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ peer, forwarded, want string }{
		{"10.20.0.2:123", "198.51.100.1", "198.51.100.1:0"},
		{"10.20.0.2:123", "198.51.100.2", "198.51.100.2:0"},
		{"192.0.2.1:123", "198.51.100.1", "192.0.2.1:123"},
		{"10.20.0.2:123", "203.0.113.99, 198.51.100.1", "198.51.100.1:0"},
		{"10.20.0.2:123", "198.51.100.1, 10.20.0.3", "198.51.100.1:0"},
		{"10.20.0.2:123", "bad-address", "10.20.0.2:123"},
		{"[2001:db8:1::2]:123", "2001:db8:2::1", "[2001:db8:2::1]:0"},
	} {
		t.Run(tc.peer+"/"+tc.forwarded, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			var got string
			middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.RemoteAddr })).ServeHTTP(httptest.NewRecorder(), r)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	for _, config := range []string{"0.0.0.0/0", "::/0", "proxy.example.com", "broken"} {
		if _, err := TrustedProxyMiddleware(config); err == nil {
			t.Fatalf("unsafe config accepted: %s", config)
		}
	}
}
