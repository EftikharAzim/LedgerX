package httptransport

import (
	"context"
	"net/http"
	"strings"

	"github.com/EftikharAzim/ledgerx/internal/service"
)

type key string

// UserIDKey identifies the authenticated user's ID in a request context.
const UserIDKey key = "user_id"

// AuthMiddleware validates a bearer access token and attaches its user ID.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(h, "Bearer ")
		userID, err := service.ParseJWT(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
