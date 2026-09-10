package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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
	ownerPlan         string
	memberCount       int
	membersList       []*db.WorkspaceMemberWithEmail
	addedMember       bool
	updatedRole       bool

	mu              sync.Mutex
	trackedChannels map[string]bool
	grantCalled     bool
	grantedChannel  string
	removedMember   bool
}

func (m *wsMockQuery) CreateWorkspace(ctx context.Context, name, createdBy string) (*db.Workspace, error) {
	if m.createdWorkspace == nil {
		m.createdWorkspace = &db.Workspace{ID: "ws-1", Name: name, CreatedBy: createdBy}
	}
	return m.createdWorkspace, nil
}

func (m *wsMockQuery) AddWorkspaceMember(ctx context.Context, workspaceID, userID, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.memberCount++
	m.addedMember = true
	return nil
}

func (m *wsMockQuery) AddWorkspaceMemberAtomic(ctx context.Context, workspaceID, userID, role string, limit int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.memberCount >= limit {
		return db.ErrSeatLimit
	}
	m.memberCount++
	m.addedMember = true
	return nil
}

func (m *wsMockQuery) UserTracksChannel(ctx context.Context, userID, channelID string) (bool, error) {
	return m.trackedChannels[channelID], nil
}

func (m *wsMockQuery) GrantChannel(ctx context.Context, workspaceID, channelID, grantedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grantCalled = true
	m.grantedChannel = channelID
	return nil
}

func (m *wsMockQuery) EnqueueLLMJobIfNotExists(ctx context.Context, commentIDs []string, provider string) (string, bool, error) {
	return "", true, nil
}

func (m *wsMockQuery) RecoverStaleLLMJobs(ctx context.Context, olderThan time.Duration) (int64, error) {
	return 0, nil
}

func (m *wsMockQuery) GetDeferredDigestsDue(ctx context.Context, now time.Time, limit int) ([]*db.WeeklyDigest, error) {
	return nil, nil
}

func (m *wsMockQuery) GetWeeklyDigestByID(ctx context.Context, digestID string) (*db.WeeklyDigest, error) {
	return nil, db.ErrNotFound
}

func (m *wsMockQuery) GetMissingDigestWeeks(ctx context.Context, userID string, weeks []time.Time) ([]time.Time, error) {
	return nil, nil
}

func (m *wsMockQuery) SetUserActiveWorkspace(ctx context.Context, userID, workspaceID string) error {
	m.activeWorkspaceID = workspaceID
	return nil
}

func (m *wsMockQuery) ListWorkspacesForUser(ctx context.Context, userID string) ([]*db.WorkspaceWithMembers, error) {
	return nil, nil
}

func (m *wsMockQuery) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]*db.WorkspaceMemberWithEmail, error) {
	return m.membersList, nil
}

func (m *wsMockQuery) CountWorkspaceMembers(ctx context.Context, workspaceID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.memberCount, nil
}

func (m *wsMockQuery) GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*db.WorkspaceMember, error) {
	if m.member == nil {
		return nil, db.ErrNotFound
	}
	if workspaceID != "ws-1" {
		return nil, db.ErrNotFound
	}
	if m.member.WorkspaceID != "" && m.member.WorkspaceID != workspaceID {
		return nil, db.ErrNotFound
	}
	return m.member, nil
}

func (m *wsMockQuery) GetWorkspace(ctx context.Context, workspaceID string) (*db.Workspace, error) {
	return &db.Workspace{ID: workspaceID, Name: "WS"}, nil
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removedMember = true
	return nil
}

func (m *wsMockQuery) UpdateWorkspaceMemberRole(ctx context.Context, workspaceID, userID, role string) error {
	m.updatedRole = true
	return nil
}

func (m *wsMockQuery) CountWorkspaceAdmins(ctx context.Context, workspaceID string) (int, error) {
	return m.adminCount, nil
}

func (m *wsMockQuery) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	return &db.User{ID: id, Email: "test@example.com", SubscriptionPlan: m.ownerPlan}, nil
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

// TestWorkspaceHandler_InviteMember_TokensRandom asserts
// fix-auth-critical-security task 3: invite tokens come from a CSPRNG, so
// two invites differ and neither equals the legacy deterministic value.
func TestWorkspaceHandler_InviteMember_TokensRandom(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	invite := func() string {
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
			t.Fatalf("expected 201, got %d (%s)", rr.Code, rr.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		url, _ := resp["invite_url"].(string)
		const prefix = "http://localhost:3000/invites/accept?token="
		token := strings.TrimPrefix(url, prefix)
		if token == "" || token == url {
			t.Fatalf("could not extract token from invite_url %q", url)
		}
		return token
	}

	legacy := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	tok1 := invite()
	tok2 := invite()
	if tok1 == tok2 {
		t.Errorf("two invites produced identical tokens %q", tok1)
	}
	if tok1 == legacy || tok2 == legacy {
		t.Errorf("invite token must not be the legacy deterministic value")
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

func TestWorkspaceHandler_RemoveMember_OwnerRejected(t *testing.T) {
	// The owner cannot be removed (no ownership-transfer endpoint exists);
	// the owner must remain a member.
	q := &wsMockQuery{
		member:     &db.WorkspaceMember{Role: "admin"},
		adminCount: 2,
		isOwner:    true,
	}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("DELETE", "/api/workspaces/ws-1/members/user-owner", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	rctx.URLParams.Add("user_id", "user-owner")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.RemoveMember(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 removing the owner, got %d (%s)", rr.Code, rr.Body.String())
	}
	if q.removedMember {
		t.Error("must not remove the workspace owner")
	}
}

func TestWorkspaceHandler_ChangeMemberRole(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}, adminCount: 2}
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
			ID:          "inv-1",
			WorkspaceID: "ws-1",
			Email:       "other@example.com",
			Role:        "viewer",
			ConsumedAt:  nil,
			ExpiresAt:   time.Now().Add(24 * time.Hour),
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

func TestWorkspaceMiddleware_ViewerDeniedWrite(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "viewer"}, activeWorkspaceID: "ws-1"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := api.WithWorkspaceID(q)(api.WorkspaceMiddleware(q)(api.RequireWorkspaceRole("admin")(next)))

	req := httptest.NewRequest("GET", "/api/workspaces/ws-1/data", nil)
	req.Header.Set("X-Workspace-Id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), api.UserIDKey, "user-1"))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestWorkspaceMiddleware_AnalystCanExport(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "analyst"}, activeWorkspaceID: "ws-1"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := api.WithWorkspaceID(q)(api.WorkspaceMiddleware(q)(api.RequireWorkspaceRole("analyst", "admin")(next)))

	req := httptest.NewRequest("GET", "/api/workspaces/ws-1/export", nil)
	req.Header.Set("X-Workspace-Id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), api.UserIDKey, "user-1"))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestWorkspaceMiddleware_AdminCanManage(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}, activeWorkspaceID: "ws-1"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := api.WithWorkspaceID(q)(api.WorkspaceMiddleware(q)(api.RequireWorkspaceRole("admin")(next)))

	req := httptest.NewRequest("GET", "/api/workspaces/ws-1/members", nil)
	req.Header.Set("X-Workspace-Id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), api.UserIDKey, "user-1"))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestWorkspaceMiddleware_NonMemberForbidden(t *testing.T) {
	q := &wsMockQuery{activeWorkspaceID: "ws-1"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := api.WithWorkspaceID(q)(api.WorkspaceMiddleware(q)(next))

	req := httptest.NewRequest("GET", "/api/workspaces/ws-1/data", nil)
	req.Header.Set("X-Workspace-Id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), api.UserIDKey, "user-1"))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_GetWorkspaceByID_Own(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/api/workspaces/ws-1", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.GetWorkspaceByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ws-1") {
		t.Errorf("expected body to contain ws-1, got %s", rr.Body.String())
	}
}

func TestWorkspaceHandler_GetWorkspaceByID_Foreign(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{WorkspaceID: "ws-1", Role: "admin"}}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/api/workspaces/ws-foreign", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-foreign")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.GetWorkspaceByID(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestWorkspaceHandler_GetWorkspaceByID_NonMember(t *testing.T) {
	q := &wsMockQuery{}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/api/workspaces/ws-1", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.GetWorkspaceByID(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// --- Seat enforcement (implement-missing-product-features 4.2 / 4.4) ---

func makeValidInvite() *db.WorkspaceInvite {
	return &db.WorkspaceInvite{
		ID:          "inv-1",
		WorkspaceID: "ws-1",
		Email:       "test@example.com",
		Role:        "viewer",
		ConsumedAt:  nil,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}
}

func TestWorkspaceHandler_AcceptInvite_SeatUnderCap(t *testing.T) {
	// Studio plan (5 seats), 4 members used -> 1 seat free: accept succeeds.
	q := &wsMockQuery{createdInvite: makeValidInvite(), ownerPlan: "studio", memberCount: 4}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/invites/accept?token=abc123", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.AcceptInvite(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !q.addedMember {
		t.Error("expected member to be added when under the seat cap")
	}
}

func TestWorkspaceHandler_AcceptInvite_SeatLimit(t *testing.T) {
	// Studio plan (5 seats), 5 members -> at cap: accept rejected.
	q := &wsMockQuery{createdInvite: makeValidInvite(), ownerPlan: "studio", memberCount: 5}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/invites/accept?token=abc123", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.AcceptInvite(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "seat limit reached") {
		t.Errorf("expected seat-limit message, got %s", rr.Body.String())
	}
	if q.addedMember {
		t.Error("must not add a member when at the seat cap")
	}
}

func TestWorkspaceHandler_ChangeMemberRole_AtSeatLimitSucceeds(t *testing.T) {
	// Free plan (1 seat), 1 member -> used == limit. A role change consumes
	// no seat, so it must still succeed (regression: the old seat-capacity
	// check wrongly blocked it).
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}, ownerPlan: "free", memberCount: 1, adminCount: 2}
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
		t.Fatalf("expected 204 at seat cap (role change consumes no seat), got %d (%s)", rr.Code, rr.Body.String())
	}
	if !q.updatedRole {
		t.Error("expected role to be updated at the seat cap")
	}
}

func TestWorkspaceHandler_ChangeMemberRole_LastAdminRejected(t *testing.T) {
	// Sole admin being demoted: must be refused so the workspace keeps an
	// admin, and the admin count / role must remain unchanged.
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}, adminCount: 1}
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

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 demoting the last admin, got %d (%s)", rr.Code, rr.Body.String())
	}
	if q.updatedRole {
		t.Error("must not change role when demoting the last admin")
	}
	if q.adminCount != 1 {
		t.Errorf("admin count changed: got %d want 1", q.adminCount)
	}
}

func TestWorkspaceHandler_ChangeMemberRole_OwnerRejected(t *testing.T) {
	// The workspace owner must never be demoted.
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}, adminCount: 2, isOwner: true}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	body := `{"role": "viewer"}`
	req := httptest.NewRequest("PATCH", "/api/workspaces/ws-1/members/user-owner", bytes.NewBufferString(body))
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	rctx.URLParams.Add("user_id", "user-owner")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.ChangeMemberRole(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 demoting the owner, got %d (%s)", rr.Code, rr.Body.String())
	}
	if q.updatedRole {
		t.Error("must not change the workspace owner's role")
	}
}

func TestWorkspaceHandler_OverCapLegacy_ExistingMembersUnaffected(t *testing.T) {
	// Free plan (1 seat) but 3 active members: legacy over-cap workspace.
	// Reads (member list) still work - no 403, no removal, no hard block.
	q := &wsMockQuery{
		member:      &db.WorkspaceMember{Role: "admin"},
		ownerPlan:   "free",
		memberCount: 3,
		membersList: []*db.WorkspaceMemberWithEmail{
			{UserID: "user-1", Email: "a@example.com", Role: "admin"},
			{UserID: "user-2", Email: "b@example.com", Role: "viewer"},
			{UserID: "user-3", Email: "c@example.com", Role: "viewer"},
		},
	}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/api/workspaces/ws-1/members", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.ListMembers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for over-cap read, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"member_count":3`) || !strings.Contains(rr.Body.String(), `"seats_limit":1`) {
		t.Errorf("expected member_count 3 and seats_limit 1, got %s", rr.Body.String())
	}
}

func TestWorkspaceHandler_OverCapLegacy_EnforcementLogsAndBlocks(t *testing.T) {
	// Over-cap legacy workspace: invite accept stays blocked (at/over cap)
	// and a warning is logged; no member is added.
	q := &wsMockQuery{createdInvite: makeValidInvite(), ownerPlan: "free", memberCount: 3}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	req := httptest.NewRequest("GET", "/invites/accept?token=abc123", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.AcceptInvite(rr, req)
	log.SetOutput(os.Stderr)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "seat limit reached") {
		t.Errorf("expected seat-limit message, got %s", rr.Body.String())
	}
	if q.addedMember {
		t.Error("must not add a member to an over-cap workspace")
	}
	if !strings.Contains(buf.String(), "over seat capacity") {
		t.Errorf("expected over-cap warning log, got %q", buf.String())
	}
}

func TestWorkspaceHandler_GetWorkspaceByID_ExposesSeats(t *testing.T) {
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "admin"}, ownerPlan: "studio", memberCount: 3}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	req := httptest.NewRequest("GET", "/api/workspaces/ws-1", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.GetWorkspaceByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"member_count":3`) || !strings.Contains(rr.Body.String(), `"seats_limit":5`) {
		t.Errorf("expected member_count 3 and seats_limit 5, got %s", rr.Body.String())
	}
}

// --- GrantChannel ownership (security fix) ---

func newGrantChannelRequest(t *testing.T, channelID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/workspaces/ws-1/channels/"+channelID+"/grant", nil)
	ctx := context.WithValue(req.Context(), api.UserIDKey, "user-1")
	req = req.WithContext(ctx)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "ws-1")
	rctx.URLParams.Add("channelId", channelID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestWorkspaceHandler_GrantChannel_UntrackedRejected(t *testing.T) {
	// An analyst may only grant channels they already legitimately track;
	// a guessed/foreign channel UUID must be refused and never inserted.
	q := &wsMockQuery{member: &db.WorkspaceMember{Role: "analyst"}}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	rr := httptest.NewRecorder()
	h.GrantChannel(rr, newGrantChannelRequest(t, "ch-foreign"))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an untracked channel, got %d (%s)", rr.Code, rr.Body.String())
	}
	if q.grantCalled {
		t.Error("must not grant a channel the actor does not track")
	}
}

func TestWorkspaceHandler_GrantChannel_TrackedSucceeds(t *testing.T) {
	q := &wsMockQuery{
		member:          &db.WorkspaceMember{Role: "analyst"},
		trackedChannels: map[string]bool{"ch-1": true},
	}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	rr := httptest.NewRecorder()
	h.GrantChannel(rr, newGrantChannelRequest(t, "ch-1"))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for a tracked channel, got %d (%s)", rr.Code, rr.Body.String())
	}
	if !q.grantCalled || q.grantedChannel != "ch-1" {
		t.Errorf("expected grant of ch-1, got called=%v channel=%q", q.grantCalled, q.grantedChannel)
	}
}

// --- AcceptInvite atomic seat admission ---

func TestWorkspaceHandler_AcceptInvite_ConcurrentSeatLimit(t *testing.T) {
	// Studio plan (5 seats), 4 members, two valid invites accepted at the
	// same time: exactly one may consume the last seat; the other gets 409.
	// The atomic guarded insert is the authority, so the cap is never
	// exceeded (regression: check-then-act admitted both).
	q := &wsMockQuery{
		createdInvite: makeValidInvite(),
		ownerPlan:     "studio",
		memberCount:   4,
	}
	h := &api.WorkspaceHandler{Queries: q, Cfg: &config.Config{AppBaseURL: "http://localhost:3000"}}

	type result struct {
		code int
		body string
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, uid := range []string{"user-a", "user-b"} {
		wg.Add(1)
		go func(uid string) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/invites/accept?token=tok-"+uid, nil)
			req = req.WithContext(context.WithValue(req.Context(), api.UserIDKey, uid))
			rr := httptest.NewRecorder()
			h.AcceptInvite(rr, req)
			results <- result{code: rr.Code, body: rr.Body.String()}
		}(uid)
	}
	wg.Wait()
	close(results)

	var redirects, conflicts int
	for r := range results {
		switch r.code {
		case http.StatusFound:
			redirects++
		case http.StatusConflict:
			conflicts++
			if !strings.Contains(r.body, "seat limit reached") {
				t.Errorf("expected seat-limit message, got %s", r.body)
			}
		default:
			t.Errorf("unexpected status %d (%s)", r.code, r.body)
		}
	}
	if redirects != 1 || conflicts != 1 {
		t.Fatalf("expected exactly one 302 and one 409, got %d redirects and %d conflicts", redirects, conflicts)
	}

	q.mu.Lock()
	got := q.memberCount
	q.mu.Unlock()
	if got != 5 {
		t.Fatalf("expected exactly 5 members after concurrent accepts, got %d", got)
	}
}
