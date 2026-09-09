package api_test

import (
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

const sentimentChannelUUID = "11111111-1111-4111-8111-111111111111"

func newSentimentRouter(h *api.InsightsHandler, userID string) *chi.Mux {
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
		r.Get("/insights/sentiment", h.Sentiment)
	})
	return r
}

// setupSentimentDB seeds a user, a channel, and tracking for it.
func setupSentimentDB(t *testing.T) (*db.SQLiteQuery, string) {
	t.Helper()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)

	userID := "sentiment-user-" + t.Name()
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO users (id, email) VALUES (?, ?)`, userID, userID+"@test.com")
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'Sentiment Channel')`,
		sentimentChannelUUID, sentimentChannelUUID)
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES (?, ?, ?, 'enc', 'enc')`,
		"sentiment-tracking-"+t.Name(), userID, sentimentChannelUUID)
	return q, userID
}

func insertSentimentComment(t *testing.T, q *db.SQLiteQuery, id, platformCommentID string, score int, postedAt time.Time) {
	t.Helper()
	_, err := q.DB().ExecContext(context.Background(),
		`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, posted_at, created_at, sentiment_score)
		 VALUES (?, ?, ?, 'body', ?, ?, ?)`,
		id, sentimentChannelUUID, platformCommentID, postedAt.Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), score)
	if err != nil {
		t.Fatalf("insert sentiment comment: %v", err)
	}
}

func getSentiment(t *testing.T, r *chi.Mux, url string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	return w, body
}

func TestSentiment_ReturnsPerDayAggregates(t *testing.T) {
	q, userID := setupSentimentDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newSentimentRouter(h, userID)

	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// Today: 2 negative, 3 neutral, 5 positive → score = -2 + 0 + 5 = 3.
	insertSentimentComment(t, q, "c-neg-1", "pc-neg-1", -1, now)
	insertSentimentComment(t, q, "c-neg-2", "pc-neg-2", -1, now)
	insertSentimentComment(t, q, "c-neu-1", "pc-neu-1", 0, now)
	insertSentimentComment(t, q, "c-neu-2", "pc-neu-2", 0, now)
	insertSentimentComment(t, q, "c-neu-3", "pc-neu-3", 0, now)
	insertSentimentComment(t, q, "c-pos-1", "pc-pos-1", 1, now)
	insertSentimentComment(t, q, "c-pos-2", "pc-pos-2", 1, now)
	insertSentimentComment(t, q, "c-pos-3", "pc-pos-3", 1, now)
	insertSentimentComment(t, q, "c-pos-4", "pc-pos-4", 1, now)
	insertSentimentComment(t, q, "c-pos-5", "pc-pos-5", 1, now)
	// Yesterday: 1 positive.
	insertSentimentComment(t, q, "c-y-pos", "pc-y-pos", 1, now.AddDate(0, 0, -1))

	w, body := getSentiment(t, r, "/api/insights/sentiment?channel_id="+sentimentChannelUUID)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if body["days"].(float64) != 7 {
		t.Errorf("days = %v, want 7", body["days"])
	}
	series, ok := body["series"].([]any)
	if !ok || len(series) != 2 {
		t.Fatalf("expected 2 series entries, got %v", body["series"])
	}
	byDate := map[string]map[string]any{}
	for _, s := range series {
		row := s.(map[string]any)
		byDate[row["date"].(string)] = row
	}
	todayRow := byDate[today]
	if todayRow == nil {
		t.Fatalf("missing today row, got dates %v", byDate)
	}
	if todayRow["negative"].(float64) != 2 || todayRow["neutral"].(float64) != 3 || todayRow["positive"].(float64) != 5 || todayRow["score"].(float64) != 3 {
		t.Errorf("today row = %v", todayRow)
	}
	yRow := byDate[yesterday]
	if yRow == nil {
		t.Fatalf("missing yesterday row, got dates %v", byDate)
	}
	if yRow["positive"].(float64) != 1 || yRow["score"].(float64) != 1 {
		t.Errorf("yesterday row = %v", yRow)
	}
}

func TestSentiment_NoScoredComments_EmptySeries(t *testing.T) {
	q, userID := setupSentimentDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newSentimentRouter(h, userID)

	w, body := getSentiment(t, r, "/api/insights/sentiment?channel_id="+sentimentChannelUUID)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	series, ok := body["series"].([]any)
	if !ok || len(series) != 0 {
		t.Errorf("expected empty series, got %v", body["series"])
	}
	if body["days"].(float64) != 7 {
		t.Errorf("days = %v, want default 7", body["days"])
	}
}

func TestSentiment_NoChannelID_GlobalAggregate(t *testing.T) {
	q, userID := setupSentimentDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newSentimentRouter(h, userID)

	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	insertSentimentComment(t, q, "g-neg-1", "g-pc-neg-1", -1, now)
	insertSentimentComment(t, q, "g-pos-1", "g-pc-pos-1", 1, now)
	insertSentimentComment(t, q, "g-pos-2", "g-pc-pos-2", 1, now)

	// Bare request and the "all" sentinel are equivalent global queries.
	for _, url := range []string{"/api/insights/sentiment", "/api/insights/sentiment?channel_id=all"} {
		w, body := getSentiment(t, r, url)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", url, w.Code, w.Body.String())
		}
		series, ok := body["series"].([]any)
		if !ok || len(series) != 1 {
			t.Fatalf("%s: expected 1 global series entry, got %v", url, body["series"])
		}
		row := series[0].(map[string]any)
		if row["date"].(string) != today {
			t.Errorf("%s: date = %v, want %v", url, row["date"], today)
		}
		if row["negative"].(float64) != 1 || row["positive"].(float64) != 2 || row["score"].(float64) != 1 {
			t.Errorf("%s: global row = %v, want negative=1 positive=2 score=1", url, row)
		}
	}
}

func TestSentiment_InvalidChannelID(t *testing.T) {
	q, userID := setupSentimentDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newSentimentRouter(h, userID)

	w, body := getSentiment(t, r, "/api/insights/sentiment?channel_id=not-a-uuid")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if body["error"] != "invalid_channel_id" {
		t.Errorf("error = %v", body["error"])
	}
}

func TestSentiment_UntrackedChannelForbidden(t *testing.T) {
	q, userID := setupSentimentDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newSentimentRouter(h, userID)

	other := "22222222-2222-4222-8222-222222222222"
	_, _ = q.DB().ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'Other')`,
		other, other)

	w, body := getSentiment(t, r, "/api/insights/sentiment?channel_id="+other)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if body["error"] != "not_tracked" {
		t.Errorf("error = %v", body["error"])
	}
}

func TestSentiment_InvalidDays(t *testing.T) {
	q, userID := setupSentimentDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newSentimentRouter(h, userID)

	for _, days := range []string{"1", "14", "100", "abc"} {
		w, body := getSentiment(t, r, "/api/insights/sentiment?channel_id="+sentimentChannelUUID+"&days="+days)
		if w.Code != http.StatusBadRequest {
			t.Errorf("days=%s: expected 400, got %d", days, w.Code)
		}
		if body["error"] != "invalid_days" {
			t.Errorf("days=%s: error = %v", days, body["error"])
		}
	}
}

func TestSentiment_ValidDaysValues(t *testing.T) {
	q, userID := setupSentimentDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newSentimentRouter(h, userID)

	wantDays := map[string]float64{"7": 7, "30": 30, "90": 90}
	for _, days := range []string{"7", "30", "90"} {
		w, body := getSentiment(t, r, "/api/insights/sentiment?channel_id="+sentimentChannelUUID+"&days="+days)
		if w.Code != http.StatusOK {
			t.Errorf("days=%s: expected 200, got %d", days, w.Code)
		}
		if got := body["days"].(float64); got != wantDays[days] {
			t.Errorf("days=%s: echoed %v", days, got)
		}
	}
}

func TestSentiment_DaysWindowFiltersOldComments(t *testing.T) {
	q, userID := setupSentimentDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newSentimentRouter(h, userID)

	// 30 days ago: outside the default 7-day window, inside the 90-day one.
	insertSentimentComment(t, q, "c-old-pos", "pc-old-pos", 1, time.Now().UTC().AddDate(0, 0, -30))

	_, body := getSentiment(t, r, "/api/insights/sentiment?channel_id="+sentimentChannelUUID)
	if series, ok := body["series"].([]any); !ok || len(series) != 0 {
		t.Errorf("default 7d window should exclude the old comment, got %v", body["series"])
	}

	_, body90 := getSentiment(t, r, "/api/insights/sentiment?channel_id="+sentimentChannelUUID+"&days=90")
	series90, ok := body90["series"].([]any)
	if !ok || len(series90) != 1 {
		t.Fatalf("90d window should include the old comment, got %v", body90["series"])
	}
	row := series90[0].(map[string]any)
	if row["positive"].(float64) != 1 || row["score"].(float64) != 1 {
		t.Errorf("old comment row = %v", row)
	}
}

func TestSentiment_AccessControlIsolation(t *testing.T) {
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)

	user1 := "sentiment-user-1"
	user2 := "sentiment-user-2"
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO users (id, email) VALUES (?, ?), (?, ?)`, user1, user1+"@test.com", user2, user2+"@test.com")

	// user1 tracks sentimentChannelUUID; user2 tracks a different channel.
	other := "33333333-3333-4333-8333-333333333333"
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'A'), (?, 'youtube', ?, 'B')`,
		sentimentChannelUUID, sentimentChannelUUID, other, other)
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES (?, ?, ?, 'enc', 'enc'), (?, ?, ?, 'enc', 'enc')`,
		"track-1", user1, sentimentChannelUUID, "track-2", user2, other)

	// user2's channel has positive comments; user1 must not see them.
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, posted_at, created_at, sentiment_score) VALUES (?, ?, ?, 'body', ?, ?, ?)`,
		"c-other-pos", other, "pc-other-pos", time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), 1)

	h := &api.InsightsHandler{Queries: q}
	r := newSentimentRouter(h, user1)

	w, body := getSentiment(t, r, "/api/insights/sentiment?channel_id="+sentimentChannelUUID)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for own channel, got %d", w.Code)
	}
	if series, ok := body["series"].([]any); !ok || len(series) != 0 {
		t.Errorf("user1 must see no comments, got %v", body["series"])
	}

	// user1 cannot query user2's channel at all.
	w2, body2 := getSentiment(t, r, "/api/insights/sentiment?channel_id="+other)
	if w2.Code != http.StatusForbidden {
		t.Errorf("expected 403 for foreign channel, got %d", w2.Code)
	}
	if body2["error"] != "not_tracked" {
		t.Errorf("error = %v", body2["error"])
	}
}
