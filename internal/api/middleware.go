package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"noumena/internal/auth"
	"noumena/internal/config"
	"noumena/internal/db"
)

type contextKey string

const (
	UserIDKey       contextKey = "userID"
	UserKey         contextKey = "user"
	TokenVersionKey contextKey = "tokenVersion"
)

// AuthMiddleware validates the JWT from the Authorization header and ensures
// the embedded token_version still matches the user's current value. It
// attaches the user_id (and token_version) to the request context.
//
// When queries is non-nil, the middleware enforces token_version. When nil
// (legacy callers), it falls back to JWT-only validation.
func AuthMiddleware(next http.Handler) http.Handler {
	return authMiddleware(nil, next)
}

// AuthMiddlewareWithQueries returns the middleware bound to a DBQuery so it
// can validate token_version against the database.
func AuthMiddlewareWithQueries(q db.DBQuery) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return authMiddleware(q, next)
	}
}

func authMiddleware(q db.DBQuery, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "authorization header missing", http.StatusUnauthorized)
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}
		claims, err := auth.ParseAccessToken(parts[1])
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		if q != nil {
			currentVersion, err := q.GetUserTokenVersion(r.Context(), claims.UserID)
			if err != nil || currentVersion != claims.TokenVersion {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
		}

		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, TokenVersionKey, claims.TokenVersion)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireVerifiedEmail rejects authenticated users whose email has not yet
// been verified. It must be installed *after* AuthMiddleware on the router.
func RequireVerifiedEmail(q db.DBQuery) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !config.Default.EmailVerificationRequired {
				next.ServeHTTP(w, r)
				return
			}
			userID, ok := r.Context().Value(UserIDKey).(string)
			if !ok || userID == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			user, err := q.GetUserByID(r.Context(), userID)
			if err != nil || user == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if user.EmailVerifiedAt == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"code": "email_not_verified"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
