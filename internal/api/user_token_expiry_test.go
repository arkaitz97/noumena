package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/dbtest"
)

// tokenExpiryMockQuery is a minimal db.DBQuery double: only the surface
// GET /api/user/token-expiry touches is implemented.
type tokenExpiryMockQuery struct {
	db.DBQuery
	channels []db.ChannelListItem
	userID   string
	err      error
}

func (m *tokenExpiryMockQuery) ListUserChannels(ctx context.Context, userID string) ([]db.ChannelListItem, error) {
	m.userID = userID
	if m.err != nil {
		return nil, m.err
	}
	return m.channels, nil
}

// newTokenExpiryRouter mirrors newSyncStatusRouter / newDigestRouter:
// inject UserIDKey directly so the test exercises handler logic, not the
// auth middleware chain.
func newTokenExpiryRouter(h *api.UserHandler, userID string) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if userID != "" {
					ctx := context.WithValue(req.Context(), api.UserIDKey, userID)
					req = req.WithContext(ctx)
				}
				next.ServeHTTP(w, req)
			})
		})
		r.Get("/user/token-expiry", h.GetTokenExpiry)
	})
	return r
}

func timePtr(t time.Time) *time.Time { return &t }

func TestGetTokenExpiry_Unauthorized(t *testing.T) {
	h := &api.UserHandler{Queries: &tokenExpiryMockQuery{}, Cfg: config.Default}
	r := newTokenExpiryRouter(h, "")

	req := httptest.NewRequest(http.MethodGet, "/api/user/token-expiry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGetTokenExpiry_FiltersToWarningWindow covers task 7.3: only tokens
// expiring inside the window surface; far-future, already-expired, null
// and non-active/paused channels are excluded, and the list is ordered by
// expiry.
func TestGetTokenExpiry_FiltersToWarningWindow(t *testing.T) {
	now := time.Now().UTC()
	mock := &tokenExpiryMockQuery{
		channels: []db.ChannelListItem{
			{ID: "pc-far", DisplayName: "Far Channel", TrackingStatus: "active", TokenExpiresAt: timePtr(now.Add(30 * 24 * time.Hour))},
			{ID: "pc-expired", DisplayName: "Expired Channel", TrackingStatus: "active", TokenExpiresAt: timePtr(now.Add(-24 * time.Hour))},
			{ID: "pc-nil", DisplayName: "Null Channel", TrackingStatus: "active", TokenExpiresAt: nil},
			{ID: "pc-paused", DisplayName: "Paused Channel", TrackingStatus: "paused", TokenExpiresAt: timePtr(now.Add(5 * 24 * time.Hour))},
			{ID: "pc-other", DisplayName: "Other Status", TrackingStatus: "detached", TokenExpiresAt: timePtr(now.Add(2 * 24 * time.Hour))},
			{ID: "pc-near", DisplayName: "Near Channel", TrackingStatus: "active", TokenExpiresAt: timePtr(now.Add(3 * 24 * time.Hour))},
		},
	}
	cfg := &config.Config{TokenExpiryWarningDays: 7}
	h := &api.UserHandler{Queries: mock, Cfg: cfg}
	r := newTokenExpiryRouter(h, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/api/user/token-expiry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if mock.userID != "user-1" {
		t.Fatalf("expected user-1 passed to store, got %q", mock.userID)
	}

	var resp struct {
		Channels    []map[string]interface{} `json:"channels"`
		WarningDays int                      `json:"warning_days"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.WarningDays != 7 {
		t.Fatalf("expected warning_days 7, got %d", resp.WarningDays)
	}
	if len(resp.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %d: %s", len(resp.Channels), w.Body.String())
	}
	if resp.Channels[0]["channel_id"] != "pc-near" || resp.Channels[1]["channel_id"] != "pc-paused" {
		t.Fatalf("expected [pc-near pc-paused] ordering, got %v %v", resp.Channels[0]["channel_id"], resp.Channels[1]["channel_id"])
	}
	for _, ch := range resp.Channels {
		for _, key := range []string{"channel_id", "title", "expires_at", "days_remaining"} {
			if _, ok := ch[key]; !ok {
				t.Errorf("channel entry missing key %q: %v", key, ch)
			}
		}
		if len(ch) != 4 {
			t.Errorf("channel entry has unexpected extra keys: %v", ch)
		}
		if ch["days_remaining"].(float64) < 1 {
			t.Errorf("days_remaining must be >= 1, got %v", ch["days_remaining"])
		}
	}
	if resp.Channels[0]["title"] != "Near Channel" {
		t.Errorf("expected Near Channel title, got %v", resp.Channels[0]["title"])
	}
}

func TestGetTokenExpiry_StoreError(t *testing.T) {
	mock := &tokenExpiryMockQuery{err: errors.New("boom")}
	h := &api.UserHandler{Queries: mock, Cfg: config.Default}
	r := newTokenExpiryRouter(h, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/api/user/token-expiry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGetTokenExpiry_RealDB exercises the endpoint against the real
// SQLite schema: the current user's expiring token is returned, other
// users' expiring tokens and far-future/expired tokens are not, and a
// user with no channels gets an empty (non-null) list.
func TestGetTokenExpiry_RealDB(t *testing.T) {
	ctx := context.Background()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database)

	user1, err := q.CreateUserWithPassword(ctx, "token-expiry-1@example.com", "hash")
	if err != nil {
		t.Fatalf("create user 1: %v", err)
	}
	user2, err := q.CreateUserWithPassword(ctx, "token-expiry-2@example.com", "hash")
	if err != nil {
		t.Fatalf("create user 2: %v", err)
	}
	user3, err := q.CreateUserWithPassword(ctx, "token-expiry-3@example.com", "hash")
	if err != nil {
		t.Fatalf("create user 3: %v", err)
	}

	mkChannel := func(t *testing.T, platformID, name string) string {
		t.Helper()
		pcID, _, err := q.UpsertPlatformChannel(ctx, "youtube", platformID, name)
		if err != nil {
			t.Fatalf("upsert platform channel %s: %v", platformID, err)
		}
		return pcID
	}

	now := time.Now().UTC()
	expiringID := mkChannel(t, "UC-expiry-near", "Expiring Soon")
	if _, err := q.CreateUserChannelTracking(ctx, user1.ID, expiringID, "enc", "ref", now.Add(5*24*time.Hour), 30, 0); err != nil {
		t.Fatalf("track expiring channel: %v", err)
	}
	farID := mkChannel(t, "UC-expiry-far", "Far Future")
	if _, err := q.CreateUserChannelTracking(ctx, user1.ID, farID, "enc", "ref", now.Add(60*24*time.Hour), 30, 0); err != nil {
		t.Fatalf("track far channel: %v", err)
	}
	expiredID := mkChannel(t, "UC-expiry-past", "Already Expired")
	if _, err := q.CreateUserChannelTracking(ctx, user1.ID, expiredID, "enc", "ref", now.Add(-24*time.Hour), 30, 0); err != nil {
		t.Fatalf("track expired channel: %v", err)
	}
	// user2 owns an expiring token too — must NOT leak into user1's view.
	otherID := mkChannel(t, "UC-expiry-other-user", "Other User Near")
	if _, err := q.CreateUserChannelTracking(ctx, user2.ID, otherID, "enc", "ref", now.Add(4*24*time.Hour), 30, 0); err != nil {
		t.Fatalf("track other-user channel: %v", err)
	}

	cfg := &config.Config{TokenExpiryWarningDays: 14}
	r := newTokenExpiryRouter(&api.UserHandler{Queries: q, Cfg: cfg}, user1.ID)

	req := httptest.NewRequest(http.MethodGet, "/api/user/token-expiry", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Channels []struct {
			ChannelID     string    `json:"channel_id"`
			Title         string    `json:"title"`
			ExpiresAt     time.Time `json:"expires_at"`
			DaysRemaining int       `json:"days_remaining"`
		} `json:"channels"`
		WarningDays int `json:"warning_days"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.WarningDays != 14 {
		t.Fatalf("expected warning_days 14, got %d", resp.WarningDays)
	}
	if len(resp.Channels) != 1 {
		t.Fatalf("expected exactly 1 channel for user1, got %d: %s", len(resp.Channels), w.Body.String())
	}
	ch := resp.Channels[0]
	if ch.ChannelID != expiringID {
		t.Fatalf("expected channel %s, got %s", expiringID, ch.ChannelID)
	}
	if ch.Title != "Expiring Soon" {
		t.Fatalf("expected title 'Expiring Soon', got %q", ch.Title)
	}
	if ch.ExpiresAt.IsZero() {
		t.Fatalf("expected expires_at to be set")
	}
	if ch.DaysRemaining != 5 {
		t.Fatalf("expected days_remaining 5, got %d", ch.DaysRemaining)
	}

	// user3 has no channels at all: empty non-null list.
	r3 := newTokenExpiryRouter(&api.UserHandler{Queries: q, Cfg: cfg}, user3.ID)
	w3 := httptest.NewRecorder()
	r3.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/api/user/token-expiry", nil))

	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty user, got %d", w3.Code)
	}
	if !json.Valid(w3.Body.Bytes()) {
		t.Fatalf("invalid JSON: %s", w3.Body.String())
	}
	var emptyResp struct {
		Channels []map[string]interface{} `json:"channels"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &emptyResp); err != nil {
		t.Fatalf("decode empty response: %v", err)
	}
	if len(emptyResp.Channels) != 0 {
		t.Fatalf("expected no channels for user3, got %v", emptyResp.Channels)
	}
}
