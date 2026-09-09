package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"noumena/internal/db"
)

type workspaceCtxKey struct{}

type WorkspaceContext struct {
	WorkspaceID string
	Role        string
}

func WorkspaceMiddleware(queries db.DBQuery) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := r.Context().Value(UserIDKey).(string)
			if !ok || userID == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			workspaceID, _ := r.Context().Value(workspaceIDCtxKey{}).(string)
			if workspaceID == "" {
				writeError(w, http.StatusBadRequest, "workspace ID required")
				return
			}

			member, err := queries.GetWorkspaceMember(r.Context(), workspaceID, userID)
			if err != nil {
				if errors.Is(err, db.ErrNotFound) {
					writeError(w, http.StatusForbidden, "not a member of this workspace")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to check workspace membership")
				return
			}

			ctx := context.WithValue(r.Context(), workspaceCtxKey{}, &WorkspaceContext{
				WorkspaceID: workspaceID,
				Role:        member.Role,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireWorkspaceRole(roles ...string) func(http.Handler) http.Handler {
	roleMap := make(map[string]bool)
	for _, r := range roles {
		roleMap[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wsCtx, ok := r.Context().Value(workspaceCtxKey{}).(*WorkspaceContext)
			if !ok || wsCtx == nil {
				writeError(w, http.StatusForbidden, "workspace context required")
				return
			}

			if !roleMap[wsCtx.Role] {
				writeError(w, http.StatusForbidden, "insufficient role")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type workspaceIDCtxKey struct{}

func WithWorkspaceID(queries db.DBQuery) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspaceID := ""
			if strings.HasPrefix(r.URL.Path, "/api/workspaces/") || strings.Contains(r.URL.Path, "/workspaces/") {
				workspaceID = chi.URLParam(r, "id")
			}
			if workspaceID == "" {
				workspaceID = r.Header.Get("X-Workspace-Id")
			}
			if workspaceID == "" {
				workspaceID = r.URL.Query().Get("workspace_id")
			}
			if workspaceID == "" {
				userID, ok := r.Context().Value(UserIDKey).(string)
				if ok && userID != "" {
					awID, err := queries.GetUserActiveWorkspaceID(r.Context(), userID)
					if err == nil && awID != "" {
						workspaceID = awID
					}
				}
			}

			ctx := context.WithValue(r.Context(), workspaceIDCtxKey{}, workspaceID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
