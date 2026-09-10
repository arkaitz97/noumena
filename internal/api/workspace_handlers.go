package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"noumena/internal/billing"
	"noumena/internal/config"
	"noumena/internal/db"
)

type WorkspaceHandler struct {
	Queries db.DBQuery
	Cfg     *config.Config
}

// channelAccessChecker is the concrete store's channel-tracking probe.
// UserTracksChannel lives on db.InsightsQuery rather than the broad DBQuery
// surface, so the handler asserts it locally and fails closed when absent.
type channelAccessChecker interface {
	UserTracksChannel(ctx context.Context, userID, channelID string) (bool, error)
}

// seatAdmitter is the concrete store's atomic, seat-guarded admission. It is
// deliberately not part of db.DBQuery so the broad mock surface is unchanged.
type seatAdmitter interface {
	AddWorkspaceMemberAtomic(ctx context.Context, workspaceID, userID, role string, limit int) error
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

	for _, w := range workspaces {
		owner, err := h.Queries.GetUserByID(r.Context(), w.CreatedBy)
		if err != nil {
			continue
		}
		w.SeatsLimit = billing.LimitsFor(owner.SubscriptionPlan).Seats
	}

	writeJSON(w, http.StatusOK, workspaces)
}

// ActiveWorkspaceRole returns the caller's role in their active workspace.
// Backs the frontend role bootstrap (GET /api/workspaces/active/role).
func (h *WorkspaceHandler) ActiveWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	wsCtx, ok := r.Context().Value(workspaceCtxKey{}).(*WorkspaceContext)
	if !ok || wsCtx == nil {
		writeError(w, http.StatusForbidden, "workspace context required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"role": wsCtx.Role})
}

// ListMembers returns the workspace member roster with emails. Admin-only
// (same gate as invite/role/remove mutations).
func (h *WorkspaceHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
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

	members, err := h.Queries.ListWorkspaceMembers(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	seats, err := h.seatUsageFor(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load seat usage")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"members":      members,
		"member_count": seats.Used,
		"seats_limit":  seats.Limit,
	})
}

func (h *WorkspaceHandler) GetWorkspaceByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	workspaceID := chi.URLParam(r, "id")

	if _, err := h.Queries.GetWorkspaceMember(r.Context(), workspaceID, userID); err != nil {
		writeWorkspaceError(w, err, http.StatusNotFound, "workspace not found", "failed to get workspace")
		return
	}

	workspace, err := h.Queries.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeWorkspaceError(w, err, http.StatusNotFound, "workspace not found", "failed to get workspace")
		return
	}

	seats, err := h.seatUsageFor(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load seat usage")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		*db.Workspace
		MemberCount int `json:"member_count"`
		SeatsLimit  int `json:"seats_limit"`
	}{workspace, seats.Used, seats.Limit})
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

	if err := h.Queries.InsertAuditLog(r.Context(), userID, "workspace.created", `{"workspace_id":"`+workspace.ID+`","name":"`+req.Name+`"}`, clientIP(r, h.Cfg)); err != nil {
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

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.updated", `{"workspace_id":"`+workspaceID+`"}`, clientIP(r, h.Cfg))
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

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.deleted", `{"workspace_id":"`+workspaceID+`"}`, clientIP(r, h.Cfg))
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

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.activated", `{"workspace_id":"`+workspaceID+`"}`, clientIP(r, h.Cfg))
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

	token, err := generateInviteToken()
	if err != nil {
		log.Printf("workspace: generate invite token: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create invite")
		return
	}
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
		`{"workspace_id":"`+workspaceID+`","email":"`+req.Email+`","role":"`+req.Role+`"}`, clientIP(r, h.Cfg))

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"invite_id":  invite.ID,
		"email":      invite.Email,
		"role":       invite.Role,
		"expires_at": invite.ExpiresAt,
		"invite_url": inviteURL,
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

	if err := h.checkSeatCapacity(r.Context(), invite.WorkspaceID); err != nil && !errors.Is(err, errSeatLimit) {
		writeError(w, http.StatusInternalServerError, "failed to check seat capacity")
		return
	}

	seats, err := h.seatUsageFor(r.Context(), invite.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load seat usage")
		return
	}

	admitter, ok := h.Queries.(seatAdmitter)
	if !ok {
		writeError(w, http.StatusInternalServerError, "failed to join workspace")
		return
	}
	if err := admitter.AddWorkspaceMemberAtomic(r.Context(), invite.WorkspaceID, userID, invite.Role, seats.Limit); err != nil {
		if errors.Is(err, db.ErrSeatLimit) {
			writeError(w, http.StatusConflict, "seat limit reached, upgrade your plan")
			return
		}
		log.Printf("workspace: atomic member admission failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to join workspace")
		return
	}

	if err := h.Queries.ConsumeWorkspaceInvite(r.Context(), invite.ID); err != nil {
	}

	if err := h.Queries.SetUserActiveWorkspace(r.Context(), userID, invite.WorkspaceID); err != nil {
	}

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.invite_accepted",
		`{"workspace_id":"`+invite.WorkspaceID+`","email":"`+invite.Email+`"}`, clientIP(r, h.Cfg))

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

	isOwner, err := h.Queries.IsWorkspaceOwner(r.Context(), workspaceID, targetUserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check workspace owner")
		return
	}
	if isOwner {
		writeError(w, http.StatusBadRequest, "cannot remove the workspace owner")
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
		`{"workspace_id":"`+workspaceID+`","target_user_id":"`+targetUserID+`"}`, clientIP(r, h.Cfg))

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

	targetMember, err := h.Queries.GetWorkspaceMember(r.Context(), workspaceID, targetUserID)
	if err != nil {
		writeWorkspaceError(w, err, http.StatusNotFound, "member not found", "failed to get workspace member")
		return
	}

	if req.Role != "admin" {
		isOwner, err := h.Queries.IsWorkspaceOwner(r.Context(), workspaceID, targetUserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check workspace owner")
			return
		}
		if isOwner {
			writeError(w, http.StatusBadRequest, "cannot change the workspace owner's role")
			return
		}
		if targetMember.Role == "admin" {
			adminCount, err := h.Queries.CountWorkspaceAdmins(r.Context(), workspaceID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to check admin count")
				return
			}
			if adminCount <= 1 {
				writeError(w, http.StatusBadRequest, "cannot demote the last admin")
				return
			}
		}
	}

	if err := h.Queries.UpdateWorkspaceMemberRole(r.Context(), workspaceID, targetUserID, req.Role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update role")
		return
	}

	_ = h.Queries.InsertAuditLog(r.Context(), userID, "workspace.member_role_changed",
		`{"workspace_id":"`+workspaceID+`","target_user_id":"`+targetUserID+`","role":"`+req.Role+`"}`, clientIP(r, h.Cfg))

	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) GrantChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	workspaceID := chi.URLParam(r, "id")
	channelID := chi.URLParam(r, "channelId")

	member, err := h.Queries.GetWorkspaceMember(r.Context(), workspaceID, userID)
	if err != nil {
		writeWorkspaceError(w, err, http.StatusForbidden, "not a member of this workspace", "failed to get workspace member")
		return
	}
	if member.Role == "viewer" {
		writeError(w, http.StatusForbidden, "analyst role required")
		return
	}

	checker, ok := h.Queries.(channelAccessChecker)
	if !ok {
		writeError(w, http.StatusInternalServerError, "failed to verify channel access")
		return
	}
	tracked, err := checker.UserTracksChannel(r.Context(), userID, channelID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify channel access")
		return
	}
	if !tracked {
		writeError(w, http.StatusForbidden, "channel is not tracked by you")
		return
	}

	if err := h.Queries.GrantChannel(r.Context(), workspaceID, channelID, userID); err != nil {
		writeWorkspaceError(w, err, http.StatusNotFound, "channel not found", "failed to grant channel")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *WorkspaceHandler) RevokeChannel(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	workspaceID := chi.URLParam(r, "id")
	channelID := chi.URLParam(r, "channelId")

	member, err := h.Queries.GetWorkspaceMember(r.Context(), workspaceID, userID)
	if err != nil {
		writeWorkspaceError(w, err, http.StatusForbidden, "not a member of this workspace", "failed to get workspace member")
		return
	}
	if member.Role == "viewer" {
		writeError(w, http.StatusForbidden, "analyst role required")
		return
	}

	if err := h.Queries.RevokeChannel(r.Context(), workspaceID, channelID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke channel")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeWorkspaceError maps a workspace-layer failure: the not-found sentinel
// gets the caller-chosen status+message (404 for resources, 403 for membership),
// anything else is a 500 with failMsg.
func writeWorkspaceError(w http.ResponseWriter, err error, notFoundStatus int, notFoundMsg, failMsg string) {
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, notFoundStatus, notFoundMsg)
		return
	}
	writeError(w, http.StatusInternalServerError, failMsg)
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

// seatUsage is the resolved seat capacity for a workspace: the owner's plan
// seat limit and the number of active members consuming seats.
type seatUsage struct {
	Limit int
	Used  int
}

// seatUsageFor resolves seat capacity from the billing plan catalog (single
// source of truth: internal/billing/plans.go, LimitsFor().Seats) and the
// active-member count in workspace_members. Pending invites live in
// workspace_invites and never consume a seat.
func (h *WorkspaceHandler) seatUsageFor(ctx context.Context, workspaceID string) (*seatUsage, error) {
	ws, err := h.Queries.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	owner, err := h.Queries.GetUserByID(ctx, ws.CreatedBy)
	if err != nil {
		return nil, err
	}
	used, err := h.Queries.CountWorkspaceMembers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return &seatUsage{Limit: billing.LimitsFor(owner.SubscriptionPlan).Seats, Used: used}, nil
}

// errSeatLimit marks a workspace at or over its plan seat cap.
var errSeatLimit = errors.New("seat limit reached")

// checkSeatCapacity returns errSeatLimit when the workspace is at or over its
// plan seat cap. Over-cap (legacy, pre-deploy) workspaces are logged but their
// existing members are never removed or denied — only new admissions (invite
// accept / role change) stay blocked while over cap.
func (h *WorkspaceHandler) checkSeatCapacity(ctx context.Context, workspaceID string) error {
	su, err := h.seatUsageFor(ctx, workspaceID)
	if err != nil {
		return err
	}
	if su.Used >= su.Limit {
		if su.Used > su.Limit {
			log.Printf("workspace %s is over seat capacity (%d/%d) - existing members unaffected, new admissions blocked",
				workspaceID, su.Used, su.Limit)
		}
		return errSeatLimit
	}
	return nil
}

// generateInviteToken returns a 32-byte cryptographically random token,
// hex-encoded (64 characters). Only the SHA-256 hash is stored; the raw value
// appears in the invite URL.
func generateInviteToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func clientIP(r *http.Request, cfg *config.Config) string {
	header := ""
	if cfg != nil {
		header = cfg.TrustProxyHeader
	}
	return trustedClientIP(r, header)
}
