package httptransport

import (
    "net/http"
    "os"
    "strings"
)

// CORSMiddleware provides CORS headers; in dev it reflects Origin, in prod use CORS_ALLOWED_ORIGINS.
// CORS_ALLOWED_ORIGINS can be a comma-separated list or "*". Avoid credentials by default.
func CORSMiddleware(next http.Handler) http.Handler {
    allowed := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        if origin == "" {
            next.ServeHTTP(w, r)
            return
        }

        allow := ""
        switch {
        case allowed == "*":
            allow = "*"
        case allowed != "":
            for _, o := range strings.Split(allowed, ",") {
                if strings.EqualFold(strings.TrimSpace(o), origin) {
                    allow = origin
                    break
                }
            }
        default:
            // Dev default: reflect origin
            allow = origin
        }

        if allow != "" {
            w.Header().Set("Access-Control-Allow-Origin", allow)
            w.Header().Set("Vary", "Origin")
            w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
            w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
        }

        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }

        next.ServeHTTP(w, r)
    })
}
