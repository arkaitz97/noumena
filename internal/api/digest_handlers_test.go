package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"noumena/internal/api"
	"noumena/internal/db"
)

// newDigestRouter mirrors newSyncStatusRouter: inject UserIDKey directly so
// the test exercises handler logic, not the auth middleware chain.
func newDigestRouter(h *api.DigestHandler, userID string) *chi.Mux {
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
		r.Get("/digests", h.List)
		r.Get("/digests/latest", h.Latest)
		r.Patch("/user/preferences", h.UpdatePreferences)
	})
	return r
}

func TestDigestList_HappyPath(t *testing.T) {
	mock := &MockQuery{
		digestsByUser: map[string][]*db.WeeklyDigest{
			"user-1": {
				{ID: "d1", UserID: "user-1", Status: "completed"},
				{ID: "d2", UserID: "user-1", Status: "completed"},
			},
		},
	}
	r := newDigestRouter(&api.DigestHandler{Queries: mock}, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/api/digests", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Digests []*db.WeeklyDigest `json:"digests"`
		Limit   int                `json:"limit"`
		Offset  int                `json:"offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(resp.Digests) != 2 {
		t.Fatalf("expected 2 digests got %d", len(resp.Digests))
	}
	if resp.Limit != 10 || resp.Offset != 0 {
		t.Fatalf("default limit/offset wrong: %d/%d", resp.Limit, resp.Offset)
	}
}

func TestDigestList_LimitClamping(t *testing.T) {
	mock := &MockQuery{}
	r := newDigestRouter(&api.DigestHandler{Queries: mock}, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/api/digests?limit=999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	var resp struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Limit != 50 {
		t.Fatalf("expected limit clamped to 50 got %d", resp.Limit)
	}
}

func TestDigestList_InvalidLimit(t *testing.T) {
	r := newDigestRouter(&api.DigestHandler{Queries: &MockQuery{}}, "user-1")
	req := httptest.NewRequest(http.MethodGet, "/api/digests?limit=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", w.Code)
	}
}

func TestDigestList_Unauthenticated(t *testing.T) {
	r := newDigestRouter(&api.DigestHandler{Queries: &MockQuery{}}, "")
	req := httptest.NewRequest(http.MethodGet, "/api/digests", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", w.Code)
	}
}

func TestDigestList_CrossUserIsolation(t *testing.T) {
	// user-1 has digests; we hit the endpoint as user-2 and expect an empty list.
	mock := &MockQuery{
		digestsByUser: map[string][]*db.WeeklyDigest{
			"user-1": {{ID: "d1", UserID: "user-1"}},
		},
	}
	r := newDigestRouter(&api.DigestHandler{Queries: mock}, "user-2")
	req := httptest.NewRequest(http.MethodGet, "/api/digests", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	var resp struct {
		Digests []*db.WeeklyDigest `json:"digests"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Digests) != 0 {
		t.Fatalf("cross-user leak: got %d digests", len(resp.Digests))
	}
}

func TestDigestLatest_404(t *testing.T) {
	r := newDigestRouter(&api.DigestHandler{Queries: &MockQuery{}}, "user-1")
	req := httptest.NewRequest(http.MethodGet, "/api/digests/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", w.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["message"] != "no digest yet" {
		t.Fatalf("expected spec-defined message got %q", body["message"])
	}
}

func TestDigestLatest_HappyPath(t *testing.T) {
	mock := &MockQuery{
		latestDigests: map[string]*db.WeeklyDigest{
			"user-1": {ID: "d-latest", UserID: "user-1", Status: "completed"},
		},
	}
	r := newDigestRouter(&api.DigestHandler{Queries: mock}, "user-1")
	req := httptest.NewRequest(http.MethodGet, "/api/digests/latest", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}
	var got db.WeeklyDigest
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.ID != "d-latest" {
		t.Fatalf("wrong digest returned: %+v", got)
	}
}

func TestUpdatePreferences_HappyPath(t *testing.T) {
	mock := &MockQuery{}
	r := newDigestRouter(&api.DigestHandler{Queries: mock}, "user-1")

	body := bytes.NewBufferString(`{"digest_email_opt_in": false}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/user/preferences", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}
	if v, ok := mock.digestOptIn["user-1"]; !ok || v != false {
		t.Fatalf("expected opt-in=false persisted, got %v (ok=%v)", v, ok)
	}
}

func TestUpdatePreferences_EmptyBody(t *testing.T) {
	r := newDigestRouter(&api.DigestHandler{Queries: &MockQuery{}}, "user-1")
	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/user/preferences", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", w.Code)
	}
}

func TestUpdatePreferences_InvalidJSON(t *testing.T) {
	r := newDigestRouter(&api.DigestHandler{Queries: &MockQuery{}}, "user-1")
	body := bytes.NewBufferString(`not json`)
	req := httptest.NewRequest(http.MethodPatch, "/api/user/preferences", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", w.Code)
	}
}

func TestUpdatePreferences_Unauthenticated(t *testing.T) {
	r := newDigestRouter(&api.DigestHandler{Queries: &MockQuery{}}, "")
	body := bytes.NewBufferString(`{"digest_email_opt_in": true}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/user/preferences", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", w.Code)
	}
}

func TestUpdatePreferences_OptInTrue(t *testing.T) {
	mock := &MockQuery{}
	r := newDigestRouter(&api.DigestHandler{Queries: mock}, "user-1")

	body := bytes.NewBufferString(`{"digest_email_opt_in": true}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/user/preferences", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		DigestEmailOptIn bool `json:"digest_email_opt_in"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !resp.DigestEmailOptIn {
		t.Fatalf("expected opt_in=true in response")
	}
	if v, ok := mock.digestOptIn["user-1"]; !ok || v != true {
		t.Fatalf("expected opt_in=true persisted, got %v (ok=%v)", v, ok)
	}
}

func TestDigestList_EmptyListReturnsEmptyArray(t *testing.T) {
	mock := &MockQuery{
		digestsByUser: map[string][]*db.WeeklyDigest{},
	}
	r := newDigestRouter(&api.DigestHandler{Queries: mock}, "user-1")

	req := httptest.NewRequest(http.MethodGet, "/api/digests", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	var resp struct {
		Digests []*db.WeeklyDigest `json:"digests"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Digests == nil {
		t.Fatalf("expected empty array, got null")
	}
}
