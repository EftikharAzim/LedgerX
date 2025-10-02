package httptransport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/net/context"
)

type RateLimiter struct {
	rdb      *redis.Client
	capacity int           // max tokens in bucket
	refill   time.Duration // refill interval (per token)
}

func NewRateLimiter(rdb *redis.Client, capacity int, refill time.Duration) *RateLimiter {
	return &RateLimiter{rdb: rdb, capacity: capacity, refill: refill}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		uid, ok := ctx.Value("user_id").(int64)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		key := "ratelimit:" + strconv.FormatInt(uid, 10)

		allowed, remaining, reset := rl.allow(ctx, key)
		if !allowed {
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.capacity))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.capacity))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))

		next.ServeHTTP(w, r)
	})
}

// allow implements token bucket in Redis
func (rl *RateLimiter) allow(ctx context.Context, key string) (bool, int, int64) {
	now := time.Now().Unix()
	lua := redis.NewScript(`
	local tokens_key = KEYS[1]
	local timestamp_key = KEYS[2]

	local capacity = tonumber(ARGV[1])
	local refill_interval = tonumber(ARGV[2]) -- seconds per token
	local now = tonumber(ARGV[3])

	local tokens = tonumber(redis.call("GET", tokens_key) or capacity)
	local last_refill = tonumber(redis.call("GET", timestamp_key) or now)

	local delta = math.max(0, now - last_refill)
	local refill = math.floor(delta / refill_interval)
	tokens = math.min(capacity, tokens + refill)
	if refill > 0 then
	last_refill = now
	end

	local allowed = 0
	if tokens > 0 then
	allowed = 1
	tokens = tokens - 1
	end

	redis.call("SET", tokens_key, tokens)
	redis.call("SET", timestamp_key, last_refill)

	return {allowed, tokens, last_refill + refill_interval}
	`)

	res, err := lua.Run(ctx, rl.rdb, []string{key + ":tokens", key + ":ts"},
		rl.capacity, int(rl.refill.Seconds()), now).Result()
	if err != nil {
		return true, rl.capacity, now + int64(rl.refill.Seconds())
	}
	vals := res.([]interface{})
	allowed := vals[0].(int64) == 1
	remaining := int(vals[1].(int64))
	reset := vals[2].(int64)
	return allowed, remaining, reset
}
