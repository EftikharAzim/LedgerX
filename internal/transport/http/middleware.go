package httptransport

import (
	"context"
	"net/http"
	"strings"

	"github.com/EftikharAzim/ledgerx/internal/service"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, "missing token", 401)
			return
		}
		token := strings.TrimPrefix(h, "Bearer ")
		userID, err := service.ParseJWT(token)
		if err != nil {
			http.Error(w, "invalid token", 401)
			return
		}
		// attach userID to context for handlers
		ctx := r.Context()
		ctx = context.WithValue(ctx, "user_id", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
