package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"noumena/internal/config"
	"noumena/internal/db"
)

type WorkspaceHandler struct {
	Queries     db.DBQuery
	Cfg         *config.Config
}

func (h *WorkspaceHandler) CreateDefaultWorkspace(userID string) error {
	w, err := h.Queries.CreateWorkspace(context.Background(), "My Workspace", userID)
	if err != nil {
		return err
	}
	if err := h.Queries.AddWorkspaceMember(context.Background(), w.ID, userID, "admin"); err != nil {
		return err
	}
	return h.Queries.SetUserActiveWorkspace(context.Background(), userID, w.ID)
}

func (h *WorkspaceHandler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	workspaces, err := h.Queries.ListWorkspacesForUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}

	writeJSON(w, http.StatusOK, workspaces)
}

func (h *WorkspaceHandler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	workspace, err := h.Queries.CreateWorkspace(r.Context(), req.Name, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}

	if err := h.Queries.AddWorkspaceMember(r.Context(), workspace.ID, userID, "admin"); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}

	if err := h.Queries.SetUserActiveWorkspace(r.Context(), userID, workspace.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set active workspace")
		return
	}

	if err := h.Queries.InsertAuditLog(r.Context(), userID, "workspace.created", `{"workspace_id":"`+workspace.ID+`","name":"`+req.Name+`"}`, clientIP(r)); err != nil {
	}

	writeJSON(w, http.StatusCreated, workspace)
}

func (h *WorkspaceHandler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(string)
	workspaceID := chi.URLParam(r, "id")

	if err := h.requireAdmin(r.Context(), userID, workspaceID); err != nil {
		if err == http.ErrAbortHandler {
			writeError(w, http.StatusForbidden, "admin role required")
			return
		}
		writeError(w, http.StatusForbidden, "not a member of this workspace")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := h.Queries.UpdateWorkspace(r.Context(), workspaceID, req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update workspace")
		return
	}

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.updated", `{"workspace_id":"`+workspaceID+`"}`, clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(UserIDKey).(string)
	workspaceID := chi.URLParam(r, "id")

	if err := h.requireAdmin(r.Context(), userID, workspaceID); err != nil {
		if err == http.ErrAbortHandler {
			writeError(w, http.StatusForbidden, "admin role required")
			return
		}
		writeError(w, http.StatusForbidden, "not a member of this workspace")
		return
	}

	if err := h.Queries.DeleteWorkspace(r.Context(), workspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workspace")
		return
	}

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.deleted", `{"workspace_id":"`+workspaceID+`"}`, clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) GetMyWorkspaces(w http.ResponseWriter, r *http.Request) {
	h.ListWorkspaces(w, r)
}

func (h *WorkspaceHandler) ActivateWorkspace(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	workspaceID := chi.URLParam(r, "id")

	member, err := h.Queries.GetWorkspaceMember(r.Context(), workspaceID, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found or not a member")
		return
	}

	if err := h.Queries.SetUserActiveWorkspace(r.Context(), userID, workspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to activate workspace")
		return
	}

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.activated", `{"workspace_id":"`+workspaceID+`"}`, clientIP(r))
	writeJSON(w, http.StatusOK, map[string]string{"workspace_id": workspaceID, "role": member.Role})
}

func (h *WorkspaceHandler) InviteMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	workspaceID := chi.URLParam(r, "id")

	if err := h.requireAdmin(r.Context(), userID, workspaceID); err != nil {
		if err == http.ErrAbortHandler {
			writeError(w, http.StatusForbidden, "admin role required")
			return
		}
		writeError(w, http.StatusForbidden, "not a member of this workspace")
		return
	}

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Role == "" {
		req.Role = "viewer"
	}
	if req.Role != "admin" && req.Role != "analyst" && req.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be admin, analyst, or viewer")
		return
	}

	token := generateInviteToken()
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	invite, err := h.Queries.CreateWorkspaceInvite(r.Context(), workspaceID, req.Email, req.Role, tokenHashHex, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create invite")
		return
	}

	inviteURL := h.Cfg.AppBaseURL + "/invites/accept?token=" + token

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.invite_created",
		`{"workspace_id":"`+workspaceID+`","email":"`+req.Email+`","role":"`+req.Role+`"}`, clientIP(r))

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"invite_id":   invite.ID,
		"email":       invite.Email,
		"role":        invite.Role,
		"expires_at":  invite.ExpiresAt,
		"invite_url":  inviteURL,
	})
}

func (h *WorkspaceHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	tokenHash := sha256.Sum256([]byte(token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	invite, err := h.Queries.GetWorkspaceInviteByToken(r.Context(), tokenHashHex)
	if err != nil {
		writeError(w, http.StatusNotFound, "invalid or expired invite")
		return
	}

	if invite.ConsumedAt != nil {
		writeError(w, http.StatusGone, "invite already consumed")
		return
	}

	if time.Now().After(invite.ExpiresAt) {
		writeError(w, http.StatusGone, "invite expired")
		return
	}

	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		http.Redirect(w, r, h.Cfg.AppBaseURL+"/register?invite="+token, http.StatusFound)
		return
	}

	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if user.Email != invite.Email {
		writeError(w, http.StatusForbidden, "invite email does not match your account email")
		return
	}

	if err := h.Queries.AddWorkspaceMember(r.Context(), invite.WorkspaceID, userID, invite.Role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to join workspace")
		return
	}

	if err := h.Queries.ConsumeWorkspaceInvite(r.Context(), invite.ID); err != nil {
	}

	if err := h.Queries.SetUserActiveWorkspace(r.Context(), userID, invite.WorkspaceID); err != nil {
	}

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.invite_accepted",
		`{"workspace_id":"`+invite.WorkspaceID+`","email":"`+invite.Email+`"}`, clientIP(r))

	http.Redirect(w, r, h.Cfg.AppBaseURL+"/dashboard", http.StatusFound)
}

func (h *WorkspaceHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	workspaceID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "user_id")

	if err := h.requireAdmin(r.Context(), userID, workspaceID); err != nil {
		if err == http.ErrAbortHandler {
			writeError(w, http.StatusForbidden, "admin role required")
			return
		}
		writeError(w, http.StatusForbidden, "not a member of this workspace")
		return
	}

	adminCount, err := h.Queries.CountWorkspaceAdmins(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check admin count")
		return
	}

	targetMember, err := h.Queries.GetWorkspaceMember(r.Context(), workspaceID, targetUserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}

	if targetMember.Role == "admin" && adminCount <= 1 {
		writeError(w, http.StatusBadRequest, "cannot remove the last admin")
		return
	}

	if err := h.Queries.RemoveWorkspaceMember(r.Context(), workspaceID, targetUserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.member_removed",
		`{"workspace_id":"`+workspaceID+`","target_user_id":"`+targetUserID+`"}`, clientIP(r))

	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) ChangeMemberRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	workspaceID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "user_id")

	if err := h.requireAdmin(r.Context(), userID, workspaceID); err != nil {
		if err == http.ErrAbortHandler {
			writeError(w, http.StatusForbidden, "admin role required")
			return
		}
		writeError(w, http.StatusForbidden, "not a member of this workspace")
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Role != "admin" && req.Role != "analyst" && req.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be admin, analyst, or viewer")
		return
	}

	if err := h.Queries.UpdateWorkspaceMemberRole(r.Context(), workspaceID, targetUserID, req.Role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update role")
		return
	}

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.member_role_changed",
		`{"workspace_id":"`+workspaceID+`","target_user_id":"`+targetUserID+`","role":"`+req.Role+`"}`, clientIP(r))

	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) requireAdmin(ctx context.Context, userID, workspaceID string) error {
	member, err := h.Queries.GetWorkspaceMember(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if member.Role != "admin" {
		return http.ErrAbortHandler
	}
	return nil
}

func generateInviteToken() string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i)
	}
	return hex.EncodeToString(b)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}
