package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/db"
)

type profileMockQuery struct {
	MockQuery
	profile *db.UserProfile
}

func (m *profileMockQuery) GetUserProfile(ctx context.Context, userID string) (*db.UserProfile, error) {
	return m.profile, nil
}

func (m *profileMockQuery) UpsertUserProfile(ctx context.Context, p *db.UserProfile) (*db.UserProfile, error) {
	m.profile = p
	return p, nil
}

func newProfileRouter(h *api.ProfileHandler, userID string) *chi.Mux {
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
		r.Get("/user/profile", h.GetProfile)
		r.Patch("/user/profile", h.PatchProfile)
	})
	return r
}

func TestProfile_Unauthenticated(t *testing.T) {
	mock := &profileMockQuery{}
	h := &api.ProfileHandler{Queries: mock}
	r := newProfileRouter(h, "")

	// GET unauthenticated -> 401
	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/user/profile unauth code = %d, want 401", w.Code)
	}

	// PATCH unauthenticated -> 401
	req = httptest.NewRequest(http.MethodPatch, "/api/user/profile", bytes.NewBufferString(`{"user_type":"creator"}`))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("PATCH /api/user/profile unauth code = %d, want 401", w.Code)
	}
}

func TestProfile_GetEmptyAndPopulated(t *testing.T) {
	mock := &profileMockQuery{}
	h := &api.ProfileHandler{Queries: mock}
	r := newProfileRouter(h, "u-123")

	// 1. When no profile exists
	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET empty profile code = %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode empty profile response: %v", err)
	}
	if resp["user_id"] != "u-123" {
		t.Errorf("user_id = %v, want u-123", resp["user_id"])
	}
	if resp["display_name"] != nil {
		t.Errorf("display_name = %v, want nil", resp["display_name"])
	}
	if resp["user_type"] != nil {
		t.Errorf("user_type = %v, want nil", resp["user_type"])
	}

	// 2. When profile exists
	userType := "creator"
	goal := "understand_audience"
	mock.profile = &db.UserProfile{
		UserID:        "u-123",
		UserType:      &userType,
		PrimaryGoal:   &goal,
		ContentTopics: []string{"gaming"},
	}

	req = httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET existing profile code = %d, want 200", w.Code)
	}
	var populatedResp db.UserProfile
	if err := json.NewDecoder(w.Body).Decode(&populatedResp); err != nil {
		t.Fatalf("decode populated profile response: %v", err)
	}
	if populatedResp.UserType == nil || *populatedResp.UserType != "creator" {
		t.Errorf("user_type = %v, want creator", populatedResp.UserType)
	}
	if len(populatedResp.ContentTopics) != 1 || populatedResp.ContentTopics[0] != "gaming" {
		t.Errorf("content_topics = %v, want [gaming]", populatedResp.ContentTopics)
	}
}

func TestProfile_PatchSuccess(t *testing.T) {
	mock := &profileMockQuery{}
	h := &api.ProfileHandler{Queries: mock}
	r := newProfileRouter(h, "u-456")

	payload := `{
		"display_name": "Alice Developer",
		"user_type": "agency",
		"primary_goal": "report_to_clients",
		"primary_platform": "youtube",
		"content_topics": ["marketing", "growth"],
		"role": "analyst",
		"insight_priorities": ["questions", "sentiment"]
	}`

	req := httptest.NewRequest(http.MethodPatch, "/api/user/profile", bytes.NewBufferString(payload))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PATCH profile code = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var updated db.UserProfile
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated profile: %v", err)
	}
	if updated.DisplayName == nil || *updated.DisplayName != "Alice Developer" {
		t.Errorf("display_name = %v, want Alice Developer", updated.DisplayName)
	}
	if updated.UserType == nil || *updated.UserType != "agency" {
		t.Errorf("user_type = %v, want agency", updated.UserType)
	}
	if updated.PrimaryGoal == nil || *updated.PrimaryGoal != "report_to_clients" {
		t.Errorf("primary_goal = %v, want report_to_clients", updated.PrimaryGoal)
	}
	if len(updated.ContentTopics) != 2 {
		t.Errorf("topics len = %d, want 2", len(updated.ContentTopics))
	}
	if len(updated.InsightPriorities) != 2 {
		t.Errorf("priorities len = %d, want 2", len(updated.InsightPriorities))
	}

	// Clearing fields via empty strings
	clearPayload := `{
		"display_name": "   ",
		"user_type": "",
		"primary_goal": "  ",
		"primary_platform": ""
	}`
	clearReq := httptest.NewRequest(http.MethodPatch, "/api/user/profile", bytes.NewBufferString(clearPayload))
	clearW := httptest.NewRecorder()
	r.ServeHTTP(clearW, clearReq)

	if clearW.Code != http.StatusOK {
		t.Fatalf("PATCH profile clear code = %d, want 200; body: %s", clearW.Code, clearW.Body.String())
	}

	var cleared db.UserProfile
	if err := json.NewDecoder(clearW.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode cleared profile: %v", err)
	}
	if cleared.DisplayName != nil {
		t.Errorf("cleared display_name = %v, want nil", cleared.DisplayName)
	}
	if cleared.UserType != nil {
		t.Errorf("cleared user_type = %v, want nil", cleared.UserType)
	}
	if cleared.PrimaryGoal != nil {
		t.Errorf("cleared primary_goal = %v, want nil", cleared.PrimaryGoal)
	}
	if cleared.PrimaryPlatform != nil {
		t.Errorf("cleared primary_platform = %v, want nil", cleared.PrimaryPlatform)
	}
}

func TestProfile_ValidationFailures(t *testing.T) {
	mock := &profileMockQuery{}
	h := &api.ProfileHandler{Queries: mock}
	r := newProfileRouter(h, "u-789")

	tests := []struct {
		name    string
		payload string
	}{
		{"display_name exceeds 100 chars", `{"display_name": "` + strings.Repeat("a", 101) + `"}`},
		{"invalid user_type", `{"user_type": "invalid_type"}`},
		{"invalid primary_goal", `{"primary_goal": "unknown_goal"}`},
		{"invalid primary_platform", `{"primary_platform": "myspace"}`},
		{"invalid audience_size_bucket", `{"audience_size_bucket": "huge"}`},
		{"invalid weekly_comment_volume_bucket", `{"weekly_comment_volume_bucket": "tons"}`},
		{"invalid success_metric", `{"success_metric": "crypto_gains"}`},
		{"invalid role", `{"role": "intern"}`},
		{"invalid team_size_bucket", `{"team_size_bucket": "medium"}`},
		{"invalid publishing_frequency_bucket", `{"publishing_frequency_bucket": "whenever"}`},
		{"invalid current_workflow", `{"current_workflow": "custom_script"}`},
		{"invalid biggest_pain", `{"biggest_pain": "bad_vibes"}`},
		{"topics exceed 10 items", `{"content_topics": ["1","2","3","4","5","6","7","8","9","10","11"]}`},
		{"topic exceeds 40 chars", `{"content_topics": ["this is a very very long topic title that exceeds forty characters"]}`},
		{"empty topic string", `{"content_topics": ["   "]}`},
		{"insight_priorities exceed 3", `{"insight_priorities": ["questions", "content_ideas", "criticism", "toxicity"]}`},
		{"invalid insight_priority value", `{"insight_priorities": ["invalid_choice"]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/api/user/profile", bytes.NewBufferString(tt.payload))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s: got code %d, want 400; body: %s", tt.name, w.Code, w.Body.String())
			}
		})
	}
}
