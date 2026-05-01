package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/auth"
	"noumena/internal/db"
)

// MockQuery implements db.DBQuery for testing
type MockQuery struct {
	users map[string]*db.User
}

func (m *MockQuery) CreateUser(ctx context.Context, email, passwordHash string) (*db.User, error) {
	if m.users == nil {
		m.users = make(map[string]*db.User)
	}
	u := &db.User{
		ID:           "test-user-id",
		Email:        email,
		PasswordHash: &passwordHash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	m.users[email] = u
	return u, nil
}

func (m *MockQuery) GetUserByEmail(ctx context.Context, email string) (*db.User, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, nil // return nil error but nil user to simulate not found
}

func (m *MockQuery) GetUserByID(ctx context.Context, id string) (*db.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

func (m *MockQuery) UpdateUserSubscription(ctx context.Context, userID, plan, stripeCustomerID string, quota int) error {
	for _, u := range m.users {
		if u.ID == userID {
			u.SubscriptionPlan = plan
			if stripeCustomerID != "" {
				u.StripeCustomerID = &stripeCustomerID
			}
			u.CommentQuotaMonthly = quota
			return nil
		}
	}
	return nil
}

func (m *MockQuery) ListChannelsForUser(ctx context.Context, userID string) ([]*db.PlatformChannel, error) {
	return nil, nil
}

func (m *MockQuery) ConnectChannelTx(ctx context.Context, userID, platform, platformChannelID, displayName, accessTokenEnc, refreshTokenEnc string, tokenExpiresAt *time.Time) error {
	return nil
}

func (m *MockQuery) PauseChannelTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}

func (m *MockQuery) RemoveChannelTracking(ctx context.Context, userID, platformChannelID string) error {
	return nil
}

func (m *MockQuery) GetChannelsForSync(ctx context.Context, minutes int) ([]*db.PlatformChannel, error) {
	return nil, nil
}

func (m *MockQuery) UpdateSyncTime(ctx context.Context, id string) error {
	return nil
}

func (m *MockQuery) GetCommentFeed(ctx context.Context, userID string, limit, offset int) ([]*db.Comment, error) {
	return []*db.Comment{
		{
			ID:                "mock-comment-id",
			PlatformChannelID: "mock-channel",
			PlatformCommentID: "mock-platform-comment",
			Body:              "Mock comment",
			PostedAt:          time.Now(),
		},
	}, nil
}

func (m *MockQuery) StarComment(ctx context.Context, userID, commentID string) error {
	return nil
}

func (m *MockQuery) GetIngestionToken(ctx context.Context, platformChannelID string) (string, string, error) {
	return "", "", nil
}

func (m *MockQuery) PromoteNextIngestionToken(ctx context.Context, platformChannelID string) error {
	return nil
}

func (m *MockQuery) InsertComment(ctx context.Context, c *db.Comment) error {
	return nil
}

func (m *MockQuery) GetUnprocessedComments(ctx context.Context, limit int) ([]*db.Comment, error) {
	return nil, nil
}

func (m *MockQuery) UpdateCommentLLMData(ctx context.Context, id, category, topic string, sentiment, confidence float64) error {
	return nil
}

func (m *MockQuery) InsertWeeklyDigest(ctx context.Context, digest *db.WeeklyDigest) error {
	return nil
}

func (m *MockQuery) GetUsersForDigest(ctx context.Context) ([]*db.User, error) {
	return nil, nil
}

func (m *MockQuery) GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*db.Comment, error) {
	return nil, nil
}

func setupRouter(queries db.DBQuery) *chi.Mux {
	r := chi.NewRouter()

	authHandler := &api.AuthHandler{Queries: queries}
	channelHandler := &api.ChannelHandler{Queries: queries}
	commentHandler := &api.CommentHandler{Queries: queries}

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(api.AuthMiddleware)

		r.Get("/channels", channelHandler.List)
		r.Get("/channels/connect/youtube", channelHandler.ConnectYouTube)
		r.Post("/channels/exchange/youtube", channelHandler.ExchangeYouTubeCode)
		r.Patch("/channels/{id}/pause", channelHandler.Pause)
		r.Get("/comments", commentHandler.Feed)
		r.Post("/comments/{id}/star", commentHandler.Star)
	})

	return r
}

func TestAuthRegisterLogin(t *testing.T) {
	queries := &MockQuery{}
	r := setupRouter(queries)

	// Test Registration
	body := map[string]string{
		"email":    "test@example.com",
		"password": "Password123",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/auth/register", bytes.NewBuffer(bodyBytes))
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("Expected status 201 Created, got %v", rr.Code)
	}

	var registerResp map[string]string
	json.NewDecoder(rr.Body).Decode(&registerResp)
	if registerResp["access_token"] == "" {
		t.Errorf("Expected an access token in response")
	}

	// Test Login
	req, _ = http.NewRequest("POST", "/auth/login", bytes.NewBuffer(bodyBytes))
	rr = httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %v", rr.Code)
	}

	var loginResp map[string]string
	json.NewDecoder(rr.Body).Decode(&loginResp)
	if loginResp["access_token"] == "" {
		t.Errorf("Expected an access token in login response")
	}
}

func TestProtectedEndpoints(t *testing.T) {
	queries := &MockQuery{}
	r := setupRouter(queries)

	// 1. Unauthenticated request to /api/channels
	req, _ := http.NewRequest("GET", "/api/channels", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized, got %v", rr.Code)
	}

	// 2. Generate a valid token
	accessToken, _, _ := auth.GenerateTokens("test-user-id")

	// 3. Authenticated request to /api/channels
	req, _ = http.NewRequest("GET", "/api/channels", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %v", rr.Code)
	}
}

func TestChannelHandlers(t *testing.T) {
	queries := &MockQuery{}
	r := setupRouter(queries)
	accessToken, _, _ := auth.GenerateTokens("test-user-id")

	// Helper to make requests
	doRequest := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req, _ = http.NewRequest(method, path, bytes.NewBuffer(body))
		} else {
			req, _ = http.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	// 1. List
	rr := doRequest("GET", "/api/channels", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for List, got %v", rr.Code)
	}

	// 2. Connect YouTube (Redirects)
	rr = doRequest("GET", "/api/channels/connect/youtube", nil)
	if rr.Code != http.StatusTemporaryRedirect {
		t.Errorf("Expected 307 Redirect, got %v", rr.Code)
	}

	// 3. Exchange YouTube Code (Mocked to fail oauth due to no real code, but let's test bad request)
	rr = doRequest("POST", "/api/channels/exchange/youtube", []byte(`{"code":""}`))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request, got %v", rr.Code)
	}

	// 4. Pause
	rr = doRequest("PATCH", "/api/channels/yt123/pause", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %v", rr.Code)
	}
}

func TestCommentHandlers(t *testing.T) {
	queries := &MockQuery{}
	r := setupRouter(queries)
	accessToken, _, _ := auth.GenerateTokens("test-user-id")

	// Helper
	doRequest := func(method, path string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	// 1. Feed
	rr := doRequest("GET", "/api/comments")
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for Feed, got %v", rr.Code)
	}

	// 2. Star
	rr = doRequest("POST", "/api/comments/c123/star")
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for Star, got %v", rr.Code)
	}
}
