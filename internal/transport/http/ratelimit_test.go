package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRateLimitBucketsExpireWithoutDisablingEnforcement(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 14})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	keys := []string{"ratelimit:ip:192.0.2.45:tokens", "ratelimit:ip:192.0.2.45:ts"}
	if err := client.Del(ctx, keys...).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Del(ctx, keys...).Err() })
	handler := NewRateLimiter(client, 2, 1).IPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	blocked := false
	for range 10 {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = "192.0.2.45:12345"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		switch response.Code {
		case http.StatusNoContent:
		case http.StatusTooManyRequests:
			blocked = true
		default:
			t.Fatalf("unexpected limiter response: %d %s", response.Code, response.Body.String())
		}
	}
	if !blocked {
		t.Fatal("rapid requests bypassed rate limiting")
	}
	for _, key := range keys {
		ttl, err := client.TTL(ctx, key).Result()
		if err != nil || ttl <= 0 || ttl > 3*time.Second {
			t.Fatalf("unbounded idle key %s: %v %v", key, ttl, err)
		}
	}
}
