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

const toxicityChannelUUID = "22222222-2222-4222-8222-222222222222"

func newToxicityRouter(h *api.InsightsHandler, userID string) *chi.Mux {
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
		r.Get("/insights/toxicity-shield", h.ToxicityShield)
	})
	return r
}

func setupToxicityDB(t *testing.T) (*db.SQLiteQuery, string) {
	t.Helper()
	database := dbtest.SetupTestDB(t)
	q := db.NewSQLiteQuery(database).(*db.SQLiteQuery)

	userID := "toxicity-user-" + t.Name()
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO users (id, email) VALUES (?, ?)`, userID, userID+"@test.com")
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES (?, 'youtube', ?, 'Toxicity Channel')`,
		toxicityChannelUUID, toxicityChannelUUID)
	_, _ = database.ExecContext(context.Background(),
		`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES (?, ?, ?, 'enc', 'enc')`,
		"toxicity-tracking-"+t.Name(), userID, toxicityChannelUUID)
	return q, userID
}

func insertToxicityComment(t *testing.T, q *db.SQLiteQuery, id, platformCommentID, category string, postedAt time.Time) {
	t.Helper()
	_, err := q.DB().ExecContext(context.Background(),
		`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, category, posted_at, processed_at, created_at)
		 VALUES (?, ?, ?, 'body', ?, ?, ?, ?)`,
		id, toxicityChannelUUID, platformCommentID, category, postedAt.Format(time.RFC3339), postedAt.Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert toxicity comment: %v", err)
	}
}

func getToxicity(t *testing.T, r *chi.Mux, url string) (*httptest.ResponseRecorder, map[string]any) {
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

func TestToxicityShield_ZeroComments_ReturnsZeroNotError(t *testing.T) {
	q, userID := setupToxicityDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newToxicityRouter(h, userID)

	w, body := getToxicity(t, r, "/api/insights/toxicity-shield?channel_id="+toxicityChannelUUID)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if body["total_comments"].(float64) != 0 {
		t.Errorf("total_comments = %v, want 0", body["total_comments"])
	}
	if body["filtered_comments"].(float64) != 0 {
		t.Errorf("filtered_comments = %v, want 0", body["filtered_comments"])
	}
	if body["filtered_pct"].(float64) != 0 {
		t.Errorf("filtered_pct = %v, want 0", body["filtered_pct"])
	}
}

func TestToxicityShield_CalculatesCorrectPercentages(t *testing.T) {
	q, userID := setupToxicityDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newToxicityRouter(h, userID)

	now := time.Now().UTC()
	// Insert 10 comments: 2 toxic_criticism, 1 spam, 7 question
	insertToxicityComment(t, q, "c1", "pc1", "toxic_criticism", now)
	insertToxicityComment(t, q, "c2", "pc2", "toxic_criticism", now)
	insertToxicityComment(t, q, "c3", "pc3", "spam", now)
	for i := 4; i <= 10; i++ {
		insertToxicityComment(t, q, "c"+string(rune('0'+i)), "pc"+string(rune('0'+i)), "question", now)
	}

	w, body := getToxicity(t, r, "/api/insights/toxicity-shield?channel_id="+toxicityChannelUUID)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if body["total_comments"].(float64) != 10 {
		t.Errorf("total_comments = %v, want 10", body["total_comments"])
	}
	if body["filtered_comments"].(float64) != 3 {
		t.Errorf("filtered_comments = %v, want 3", body["filtered_comments"])
	}
	if body["filtered_pct"].(float64) != 30 {
		t.Errorf("filtered_pct = %v, want 30.0", body["filtered_pct"])
	}
	if body["window"] != "7d" {
		t.Errorf("window = %v, want 7d", body["window"])
	}
}

func TestToxicityShield_GlobalAggregate(t *testing.T) {
	q, userID := setupToxicityDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newToxicityRouter(h, userID)

	now := time.Now().UTC()
	insertToxicityComment(t, q, "c1", "pc1", "spam", now)
	insertToxicityComment(t, q, "c2", "pc2", "question", now)

	for _, url := range []string{"/api/insights/toxicity-shield", "/api/insights/toxicity-shield?channel_id=all"} {
		w, body := getToxicity(t, r, url)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", url, w.Code, w.Body.String())
		}
		if body["total_comments"].(float64) != 2 {
			t.Errorf("%s: total_comments = %v, want 2", url, body["total_comments"])
		}
		if body["filtered_comments"].(float64) != 1 {
			t.Errorf("%s: filtered_comments = %v, want 1", url, body["filtered_comments"])
		}
		if body["filtered_pct"].(float64) != 50 {
			t.Errorf("%s: filtered_pct = %v, want 50.0", url, body["filtered_pct"])
		}
	}
}

func TestToxicityShield_Windows(t *testing.T) {
	q, userID := setupToxicityDB(t)
	h := &api.InsightsHandler{Queries: q}
	r := newToxicityRouter(h, userID)

	now := time.Now().UTC()
	// c-recent: 2 days ago (in 7d, 30d, 90d) -> toxic
	insertToxicityComment(t, q, "c-recent", "pc-recent", "toxic_criticism", now.AddDate(0, 0, -2))
	// c-mid: 15 days ago (in 30d, 90d; outside 7d) -> praise
	insertToxicityComment(t, q, "c-mid", "pc-mid", "praise", now.AddDate(0, 0, -15))
	// c-old: 45 days ago (in 90d; outside 30d, 7d) -> spam
	insertToxicityComment(t, q, "c-old", "pc-old", "spam", now.AddDate(0, 0, -45))

	// 7d window: 1 comment (1 toxic) -> 100%
	w7, b7 := getToxicity(t, r, "/api/insights/toxicity-shield?channel_id="+toxicityChannelUUID+"&window=7d")
	if w7.Code != http.StatusOK {
		t.Fatalf("7d: expected 200, got %d", w7.Code)
	}
	if b7["total_comments"].(float64) != 1 || b7["filtered_comments"].(float64) != 1 || b7["filtered_pct"].(float64) != 100 {
		t.Errorf("7d: got %v", b7)
	}
	if b7["window"] != "7d" {
		t.Errorf("7d: window = %v", b7["window"])
	}

	// 30d window: 2 comments (1 toxic) -> 50%
	w30, b30 := getToxicity(t, r, "/api/insights/toxicity-shield?channel_id="+toxicityChannelUUID+"&window=30d")
	if w30.Code != http.StatusOK {
		t.Fatalf("30d: expected 200, got %d", w30.Code)
	}
	if b30["total_comments"].(float64) != 2 || b30["filtered_comments"].(float64) != 1 || b30["filtered_pct"].(float64) != 50 {
		t.Errorf("30d: got %v", b30)
	}
	if b30["window"] != "30d" {
		t.Errorf("30d: window = %v", b30["window"])
	}

	// 90d window: 3 comments (2 toxic) -> 66.7%
	w90, b90 := getToxicity(t, r, "/api/insights/toxicity-shield?channel_id="+toxicityChannelUUID+"&window=90d")
	if w90.Code != http.StatusOK {
		t.Fatalf("90d: expected 200, got %d", w90.Code)
	}
	if b90["total_comments"].(float64) != 3 || b90["filtered_comments"].(float64) != 2 || b90["filtered_pct"].(float64) != 66.7 {
		t.Errorf("90d: got %v", b90)
	}
	if b90["window"] != "90d" {
		t.Errorf("90d: window = %v", b90["window"])
	}
}
