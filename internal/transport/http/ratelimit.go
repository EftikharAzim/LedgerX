package httptransport

import (
    "context"
    "net/http"
    "strconv"
    "time"

    "github.com/redis/go-redis/v9"
)

type RateLimiter struct {
    rdb      *redis.Client
    capacity int // max tokens in bucket
    rate     int // tokens added per second
}

// NewRateLimiter creates a token bucket limiter that refills at `rate` tokens per second
// with a maximum burst capacity of `capacity`.
func NewRateLimiter(rdb *redis.Client, capacity int, rate int) *RateLimiter {
    if capacity <= 0 {
        capacity = rate
        if capacity <= 0 {
            capacity = 1
        }
    }
    if rate <= 0 {
        rate = 1
    }
    return &RateLimiter{rdb: rdb, capacity: capacity, rate: rate}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        // Use the same typed context key as the AuthMiddleware (UserIDKey)
        uid, ok := ctx.Value(UserIDKey).(int64)
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

// allow implements token bucket in Redis with integer tokens-per-second rate.
func (rl *RateLimiter) allow(ctx context.Context, key string) (bool, int, int64) {
    now := time.Now().Unix()
    lua := redis.NewScript(`
local tokens_key = KEYS[1]
local timestamp_key = KEYS[2]

local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2]) -- tokens per second
local now = tonumber(ARGV[3])

local tokens = tonumber(redis.call("GET", tokens_key) or capacity)
local last_refill = tonumber(redis.call("GET", timestamp_key) or now)

local delta = math.max(0, now - last_refill)
local refill = math.floor(delta * rate)
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

local reset = last_refill + 1 -- approximate next-second window
return {allowed, tokens, reset}
    `)

    res, err := lua.Run(ctx, rl.rdb, []string{key + ":tokens", key + ":ts"}, rl.capacity, rl.rate, now).Result()
    if err != nil {
        return true, rl.capacity, now + 1
    }
    vals := res.([]interface{})
    allowed := vals[0].(int64) == 1
    remaining := int(vals[1].(int64))
    reset := vals[2].(int64)
    return allowed, remaining, reset
}
