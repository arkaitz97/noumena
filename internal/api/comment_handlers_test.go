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
	"noumena/internal/db"
	"noumena/internal/dbtest"
)

func newInsightsRouter(h *api.InsightsHandler, userID string) *chi.Mux {
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
		r.Get("/comments/search", h.SearchComments)
		r.Post("/comments/bulk-star", h.BulkStarComments)
		r.Post("/comments/bulk-unstar", h.BulkUnstarComments)
		r.Get("/comments/{id}/thread", h.GetCommentThread)
	})
	return r
}

func setupInsightsDB(t *testing.T) (*db.SQLiteQuery, string) {
	t.Helper()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)

	userID := "test-user-" + t.Name()
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO users (id, email) VALUES (?, ?)`, userID, userID+"@test.com")

	channelID := "test-channel-" + t.Name()
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'Test Channel')`,
		channelID, channelID)

	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES (?, ?, ?, 'enc', 'enc')`,
		"tracking-"+t.Name(), userID, channelID)

	return q, userID
}

func insertComment(t *testing.T, q *db.SQLiteQuery, channelID, platformCommentID, body string, parentCommentID *string, replyDepth int) string {
	t.Helper()
	c := &db.Comment{
		PlatformChannelID: channelID,
		PlatformCommentID: platformCommentID,
		Body:              body,
		PostedAt:          time.Now(),
		ParentCommentID:   parentCommentID,
		ReplyDepth:        replyDepth,
	}
	if err := q.InsertComment(context.Background(), c); err != nil {
		t.Fatalf("insert comment: %v", err)
	}
	var id string
	if err := q.DB().QueryRowContext(context.Background(),
		`SELECT id FROM comments WHERE platform_comment_id = ?`, platformCommentID).Scan(&id); err != nil {
		t.Fatalf("get comment id: %v", err)
	}
	return id
}

func TestSearchComments_ValidQuery(t *testing.T) {
	q, userID := setupInsightsDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newInsightsRouter(h, userID)

	channelID := "search-channel"
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'Search Channel')`,
		channelID, channelID)
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES (?, ?, ?, 'enc', 'enc')`,
		"tracking-search", userID, channelID)

	insertComment(t, q, channelID, "pc-search-1", "this is a test comment about performance", nil, 0)
	insertComment(t, q, channelID, "pc-search-2", "another comment about something else", nil, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/comments/search?q=performance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v body=%s", err, w.Body.String())
	}
	comments, ok := resp["comments"].([]interface{})
	if !ok {
		t.Fatalf("expected comments array, got resp=%v", resp)
	}
	if len(comments) < 1 {
		t.Fatalf("expected at least 1 comment, got %d", len(comments))
	}
}

func TestSearchComments_ShortQuery(t *testing.T) {
	q, userID := setupInsightsDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newInsightsRouter(h, userID)

	req := httptest.NewRequest(http.MethodGet, "/api/comments/search?q=ab", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", w.Code)
	}
}

func TestBulkStarComments(t *testing.T) {
	q, userID := setupInsightsDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newInsightsRouter(h, userID)

	channelID := "bulk-star-channel"
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'Bulk Star Channel')`,
		channelID, channelID)
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES (?, ?, ?, 'enc', 'enc')`,
		"tracking-bulkstar", userID, channelID)

	id1 := insertComment(t, q, channelID, "pc-bulkstar-1", "comment one", nil, 0)
	id2 := insertComment(t, q, channelID, "pc-bulkstar-2", "comment two", nil, 0)

	body := map[string][]string{"comment_ids": {id1, id2}}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/comments/bulk-star", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	starred, ok := resp["starred"].(float64)
	if !ok || int(starred) != 2 {
		t.Fatalf("expected starred=2, got %v", resp["starred"])
	}
}

func TestBulkUnstarComments(t *testing.T) {
	q, userID := setupInsightsDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newInsightsRouter(h, userID)

	channelID := "bulk-unstar-channel"
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'Bulk Unstar Channel')`,
		channelID, channelID)
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES (?, ?, ?, 'enc', 'enc')`,
		"tracking-bulkunstar", userID, channelID)

	id1 := insertComment(t, q, channelID, "pc-bulkunstar-1", "comment one", nil, 0)
	id2 := insertComment(t, q, channelID, "pc-bulkunstar-2", "comment two", nil, 0)

	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO user_comment_stars (user_id, comment_id) VALUES (?, ?)`, userID, id1)
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO user_comment_stars (user_id, comment_id) VALUES (?, ?)`, userID, id2)

	body := map[string][]string{"comment_ids": {id1, id2}}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/comments/bulk-unstar", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	unstarred, ok := resp["unstarred"].(float64)
	if !ok || int(unstarred) != 2 {
		t.Fatalf("expected unstarred=2, got %v", resp["unstarred"])
	}
}

func TestGetCommentThread(t *testing.T) {
	q, userID := setupInsightsDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newInsightsRouter(h, userID)

	channelID := "thread-channel"
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'Thread Channel')`,
		channelID, channelID)
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES (?, ?, ?, 'enc', 'enc')`,
		"tracking-thread", userID, channelID)

	topPlatformID := "pc-top"
	reply1PlatformID := "pc-reply1"
	insertComment(t, q, channelID, topPlatformID, "top level comment", nil, 0)
	insertComment(t, q, channelID, reply1PlatformID, "first reply", &topPlatformID, 1)
	insertComment(t, q, channelID, "pc-reply2", "second reply", &topPlatformID, 1)
	insertComment(t, q, channelID, "pc-reply3", "reply to reply", &reply1PlatformID, 2)

	// Get the UUID of the top-level comment for the URL
	var topUUID string
	q.DB().QueryRowContext(context.Background(),
		`SELECT id FROM comments WHERE platform_comment_id = ?`, topPlatformID).Scan(&topUUID)

	req := httptest.NewRequest(http.MethodGet, "/api/comments/"+topUUID+"/thread", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}
	var comments []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &comments); err != nil {
		t.Fatalf("json: %v body=%s", err, w.Body.String())
	}
	if len(comments) < 4 {
		t.Fatalf("expected at least 4 comments in thread, got %d", len(comments))
	}
}

func TestGetCommentThread_AccessControl(t *testing.T) {
	q, _ := setupInsightsDB(t)
	h := &api.InsightsHandler{Queries: q}

	otherUserID := "other-user-" + t.Name()
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO users (id, email) VALUES (?, ?)`, otherUserID, otherUserID+"@test.com")

	channelID := "thread-ac-channel"
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'AC Channel')`,
		channelID, channelID)

	otherTrackingID := "tracking-other-" + t.Name()
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES (?, ?, ?, 'enc', 'enc')`,
		otherTrackingID, otherUserID, channelID)

	topPlatformID := "pc-ac-top"
	insertComment(t, q, channelID, topPlatformID, "top level", nil, 0)

	var topUUID string
	q.DB().QueryRowContext(context.Background(),
		`SELECT id FROM comments WHERE platform_comment_id = ?`, topPlatformID).Scan(&topUUID)

	rOther := newInsightsRouter(h, otherUserID)
	req := httptest.NewRequest(http.MethodGet, "/api/comments/"+topUUID+"/thread", nil)
	w := httptest.NewRecorder()
	rOther.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}

	var comments []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &comments); err != nil {
		t.Fatalf("json: %v body=%s", err, w.Body.String())
	}
	if len(comments) < 1 {
		t.Fatalf("expected at least 1 comment, got %d", len(comments))
	}
}
