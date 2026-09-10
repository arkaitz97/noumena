package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"noumena/internal/db"
	"noumena/internal/ingestion"
)

// leakedInternalError simulates a raw upstream/library error (endpoint, host,
// and payload fragment) that must never be echoed to the client.
const leakedInternalError = "SUPER_SECRET_UPSTREAM https://oauth2.googleapis.com/token: " +
	"invalid_grant payload={\"client_secret\":\"shhh\"}"

// leakMockQuery is the minimal DBQuery double needed to reach the resolver and
// exchanger failure paths: user lookup for the plan-capacity guard plus a
// consumed CSRF state.
type leakMockQuery struct {
	db.DBQuery
	user *db.User
}

func (m *leakMockQuery) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	return m.user, nil
}

func (m *leakMockQuery) GetActiveChannelCount(ctx context.Context, userID string) (int, error) {
	return 0, nil
}

func (m *leakMockQuery) ConsumeOAuthState(ctx context.Context, hash, userID string) (bool, error) {
	return true, nil
}

type stubFailingResolver struct{ err error }

func (s *stubFailingResolver) ResolveChannelRef(ctx context.Context, ref string) (*ingestion.ChannelInfo, error) {
	return nil, s.err
}

func authenticatedRequest(method, target, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	return req.WithContext(context.WithValue(req.Context(), UserIDKey, "user-1"))
}

func assertNoLeak(t *testing.T, rec *httptest.ResponseRecorder, wantCode, wantMessage string) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := rec.Body.String()
	if strings.Contains(body, leakedInternalError) {
		t.Fatalf("response leaked raw internal error: %s", body)
	}
	if strings.Contains(body, "SUPER_SECRET_UPSTREAM") {
		t.Fatalf("response leaked internal marker: %s", body)
	}
	var resp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, body)
	}
	if resp.Code != wantCode {
		t.Errorf("code = %q, want %q", resp.Code, wantCode)
	}
	if !strings.Contains(resp.Message, wantMessage) {
		t.Errorf("message = %q, want it to contain %q", resp.Message, wantMessage)
	}
}

// TestExchangeYouTubeCode_UpstreamFailureDoesNotLeakInternalError injects an
// exchanger double that returns a distinctive internal error and proves the
// handler preserves the machine-readable code and status while returning only
// a generic client message.
func TestExchangeYouTubeCode_UpstreamFailureDoesNotLeakInternalError(t *testing.T) {
	h := &ChannelHandler{
		Queries: &leakMockQuery{user: &db.User{ID: "user-1", SubscriptionPlan: "free"}},
		OAuthExchanger: func(ctx context.Context, code string) (*oauth2.Token, *ingestion.ChannelInfo, error) {
			return nil, nil, errors.New(leakedInternalError)
		},
	}
	rec := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodPost, "/api/channels/exchange/youtube",
		`{"code":"one-time-code","state":"csrf-state"}`)

	h.ExchangeYouTubeCode(rec, req)

	assertNoLeak(t, rec, "exchange_failed", "could not exchange authorization code")
}

// TestConnectYouTubeByName_ResolveFailureDoesNotLeakInternalError injects a
// resolver double that returns a distinctive internal error and proves the
// handler preserves the machine-readable code and status while returning only
// a generic client message.
func TestConnectYouTubeByName_ResolveFailureDoesNotLeakInternalError(t *testing.T) {
	h := &ChannelHandler{
		Queries:         &leakMockQuery{user: &db.User{ID: "user-1", SubscriptionPlan: "free"}},
		YouTubeResolver: &stubFailingResolver{err: errors.New(leakedInternalError)},
	}
	rec := httptest.NewRecorder()
	req := authenticatedRequest(http.MethodPost, "/api/channels/connect/youtube/by-name",
		`{"ref":"@does-not-exist"}`)

	h.ConnectYouTubeByName(rec, req)

	assertNoLeak(t, rec, "resolve_failed", "could not resolve channel")
}
