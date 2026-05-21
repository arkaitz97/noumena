package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/config"
	"noumena/internal/db"
)

type wsMockQuery struct {
	rlMockQuery
	createdWorkspace  *db.Workspace
	createdInvite     *db.WorkspaceInvite
	member            *db.WorkspaceMember
	adminCount        int
	isOwner           bool
	activeWorkspaceID string
}

func (m *wsMockQuery) CreateWorkspace(ctx context.Context, name, createdBy string) (*db.Workspace, error) {
	if m.createdWorkspace == nil {
		m.createdWorkspace = &db.Workspace{ID: "ws-1", Name: name, CreatedBy: createdBy}
	}
	return m.createdWorkspace, nil
}

func (m *wsMockQuery) AddWorkspaceMember(ctx context.Context, workspaceID, userID, role string) error {
	return nil
}

func (m *wsMockQuery) SetUserActiveWorkspace(ctx context.Context, userID, workspaceID string) error {
	m.activeWorkspaceID = workspaceID
	return nil
}

func (m *wsMockQuery) ListWorkspacesForUser(ctx context.Context, userID string) ([]*db.WorkspaceWithMembers, error) {
	return nil, nil
}

func (m *wsMockQuery) GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*db.WorkspaceMember, error) {
	if m.member != nil {
		return m.member, nil
	}
	return nil, nil
}

func (m *wsMockQuery) CreateWorkspaceInvite(ctx context.Context, workspaceID, email, role, tokenHash string, expiresAt time.Time) (*db.WorkspaceInvite, error) {
	if m.createdInvite == nil {
		m.createdInvite = &db.WorkspaceInvite{ID: "inv-1", WorkspaceID: workspaceID, Email: email, Role: role}
	}
	return m.createdInvite, nil
}

func (m *wsMockQuery) GetWorkspaceInviteByToken(ctx context.Context, tokenHash string) (*db.WorkspaceInvite, error) {
	if m.createdInvite != nil {
		return m.createdInvite, nil
	}
	return nil, nil
}

func (m *wsMockQuery) ConsumeWorkspaceInvite(ctx context.Context, inviteID string) error {
	return nil
}

func (m *wsMockQuery) RemoveWorkspaceMember(ctx context.Context, workspaceID, userID string) error {
	return nil
}

func (m *wsMockQuery) UpdateWorkspaceMemberRole(ctx context.Context, workspaceID, userID, role string) error {
	return nil
}

func (m *wsMockQuery) CountWorkspaceAdmins(ctx context.Context, workspaceID string) (int, error) {
	return m.adminCount, nil
}

func (m *wsMockQuery) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	return &db.User{ID: id, Email: "test@example.com"}, nil
}

func (m *wsMockQuery) GetUserActiveWorkspaceID(ctx context.Context, userID string) (string, error) {
	return m.activeWorkspaceID, nil
}

func (m *wsMockQuery) IsWorkspaceOwner(ctx context.Context, workspaceID, userID string) (bool, error) {
	return m.isOwner, nil
}

func TestWorkspaceHandler_CreateWorkspace(t *testing.T) {
	q := &wsMockQuery{}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	body := `{"name": "Test Workspace"}`
	req := httptest.NewRequest("POST", "/api/workspaces", bytes.NewBufferString(body))
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.CreateWorkspace(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
	if q.activeWorkspaceID != "ws-1" {
		t.Errorf("expected active workspace to be set to ws-1, got %s", q.activeWorkspaceID)
	}
}

func TestWorkspaceHandler_CreateWorkspace_MissingName(t *testing.T) {
	q := &wsMockQuery{}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	body := `{}`
	req := httptest.NewRequest("POST", "/api/workspaces", bytes.NewBufferString(body))
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.CreateWorkspace(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_CreateWorkspace_Unauthorized(t *testing.T) {
	q := &wsMockQuery{}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	body := `{"name": "Test Workspace"}`
	req := httptest.NewRequest("POST", "/api/workspaces", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.CreateWorkspace(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_InviteMember(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	body := `{"email": "new@example.com", "role": "viewer"}`
	req := httptest.NewRequest("POST", "/api/workspaces/ws-1/invites", bytes.NewBufferString(body))
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.InviteMember(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_InviteMember_NotAdmin(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "viewer"}}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	body := `{"email": "new@example.com", "role": "viewer"}`
	req := httptest.NewRequest("POST", "/api/workspaces/ws-1/invites", bytes.NewBufferString(body))
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.InviteMember(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_InviteMember_InvalidRole(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	body := `{"email": "new@example.com", "role": "superadmin"}`
	req := httptest.NewRequest("POST", "/api/workspaces/ws-1/invites", bytes.NewBufferString(body))
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.InviteMember(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_RemoveMember_LastAdmin(t *testing.T) {
	q := &wsMockQuery{
		member:     &db.WorkspaceMember{Role: "admin"},
		adminCount: 1,
	}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("DELETE", "/api/workspaces/ws-1/members/user-2", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	rctx.URLParams.Add("user_id", "user-2")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.RemoveMember(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 (cannot remove last admin), got %d", rr.Code)
	}
}

func TestWorkspaceHandler_RemoveMember_Success(t *testing.T) {
	q := &wsMockQuery{
		member:     &db.WorkspaceMember{Role: "admin"},
		adminCount: 2,
	}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("DELETE", "/api/workspaces/ws-1/members/user-2", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	rctx.URLParams.Add("user_id", "user-2")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.RemoveMember(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_ChangeMemberRole(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	body := `{"role": "analyst"}`
	req := httptest.NewRequest("PATCH", "/api/workspaces/ws-1/members/user-2", bytes.NewBufferString(body))
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	rctx.URLParams.Add("user_id", "user-2")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.ChangeMemberRole(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_AcceptInvite_EmailMismatch(t *testing.T) {
	q := &wsMockQuery{
		createdInvite: &db.WorkspaceInvite{
			ID:           "inv-1",
			WorkspaceID:  "ws-1",
			Email:        "other@example.com",
			Role:         "viewer",
			ConsumedAt:   nil,
			ExpiresAt:    time.Now().Add(24 * time.Hour),
		},
	}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/invites/accept?token=abc123", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.AcceptInvite(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 (email mismatch), got %d", rr.Code)
	}
}

func TestWorkspaceHandler_AcceptInvite_NoToken(t *testing.T) {
	q := &wsMockQuery{}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/invites/accept", nil)
	rr := httptest.NewRecorder()

	h.AcceptInvite(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_ListWorkspaces(t *testing.T) {
	q := &wsMockQuery{}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/api/workspaces", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.ListWorkspaces(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
