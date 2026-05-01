package api

import (
	"context"
	"net/http"
	"strings"

	"noumena/internal/auth"
	"noumena/internal/db"
)

type contextKey string

const UserIDKey contextKey = "userID"

// AuthMiddleware validates the JWT from the Authorization header
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "authorization header missing", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenStr := parts[1]
		claims, err := auth.ValidateToken(tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// QuotaMiddleware checks if the user has exceeded their comment processing quota.
func QuotaMiddleware(queries db.DBQuery) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := r.Context().Value(UserIDKey).(string)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			user, err := queries.GetUserByID(r.Context(), userID)
			if err != nil || user == nil {
				http.Error(w, "user not found", http.StatusNotFound)
				return
			}

			if user.CommentsUsedThisPeriod >= user.CommentQuotaMonthly {
				http.Error(w, "quota exceeded, please upgrade your billing plan", http.StatusPaymentRequired)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
