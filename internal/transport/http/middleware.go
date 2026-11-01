package httptransport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	sqlc "github.com/EftikharAzim/ledgerx/internal/repo/sqlc"
	"github.com/EftikharAzim/ledgerx/internal/service"
	"github.com/jackc/pgx/v5"
)

type key string

// UserIDKey identifies the authenticated user's ID in a request context.
const UserIDKey key = "user_id"

const authVersionKey key = "auth_version"

// AuthMiddleware validates a bearer access token and attaches its user ID.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(h, "Bearer ")
		userID, version, err := service.ParseSessionJWT(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, authVersionKey, version)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireCurrentSession revokes access immediately after password recovery,
// password change, or account deletion. It fails closed on database errors.
func RequireCurrentSession(q *sqlc.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, _ := r.Context().Value(UserIDKey).(int64)
			version, _ := r.Context().Value(authVersionKey).(int64)
			current, err := q.GetAuthVersion(r.Context(), uid)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && version != current) {
				http.Error(w, "session ended; please log in again", http.StatusUnauthorized)
				return
			}
			if err != nil {
				http.Error(w, "session check unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	}
}
