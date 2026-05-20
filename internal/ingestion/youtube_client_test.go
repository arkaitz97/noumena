package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestYouTubeClient_TokenInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer srv.Close()
	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	next := c.ListVideos(context.Background(), "tok", "ch", time.Time{})
	_, _, err := next()
	var tie *TokenInvalidError
	if !errors.As(err, &tie) {
		t.Fatalf("expected TokenInvalidError, got %v", err)
	}
}

func TestYouTubeClient_RateLimit429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	next := c.ListVideos(context.Background(), "tok", "ch", time.Time{})
	_, _, err := next()
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected RateLimitError, got %v", err)
	}
	if rle.RetryAfter != 120*time.Second {
		t.Fatalf("expected 120s retry-after, got %v", rle.RetryAfter)
	}
}

func TestYouTubeClient_QuotaExceededDefault1h(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"errors":[{"reason":"quotaExceeded"}]}}`))
	}))
	defer srv.Close()
	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	next := c.ListCommentThreads(context.Background(), "tok", "vid")
	_, _, err := next()
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("expected RateLimitError, got %v", err)
	}
	if rle.RetryAfter != time.Hour {
		t.Fatalf("expected 1h default, got %v", rle.RetryAfter)
	}
}

func TestYouTubeClient_PaginatedSearch(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			_, _ = w.Write([]byte(`{
				"nextPageToken":"P2",
				"items":[
					{"id":{"videoId":"v1"},"snippet":{"publishedAt":"2025-01-02T10:00:00Z"}},
					{"id":{"videoId":"v2"},"snippet":{"publishedAt":"2025-01-01T10:00:00Z"}}
				]}`))
		case 2:
			_, _ = w.Write([]byte(`{
				"items":[
					{"id":{"videoId":"v3"},"snippet":{"publishedAt":"2024-12-31T10:00:00Z"}}
				]}`))
		}
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	next := c.ListVideos(context.Background(), "tok", "ch", time.Time{})
	var ids []string
	for {
		v, ok, err := next()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !ok {
			break
		}
		ids = append(ids, v.ID)
	}
	if got := strings.Join(ids, ","); got != "v1,v2,v3" {
		t.Fatalf("expected v1,v2,v3 got %s", got)
	}
}

func TestYouTubeClient_ReplyMapping(t *testing.T) {
	resp := commentThreadsResponse{
		Items: []commentThreadItem{
			{
				ID: "T1",
				Snippet: struct {
					TopLevelComment ytComment `json:"topLevelComment"`
					TotalReplyCount int       `json:"totalReplyCount"`
				}{
					TopLevelComment: ytComment{
						ID: "abc",
						Snippet: commentSnippet{
							AuthorDisplayName: "Alice",
							TextDisplay:       "hi",
							PublishedAt:       "2025-01-01T10:00:00Z",
						},
					},
				},
				Replies: &struct {
					Comments []ytComment `json:"comments"`
				}{
					Comments: []ytComment{{
						ID: "xyz",
						Snippet: commentSnippet{
							AuthorDisplayName: "Bob",
							TextDisplay:       "reply",
							PublishedAt:       "2025-01-01T11:00:00Z",
							ParentID:          "abc",
						},
					}},
				},
			},
		},
	}
	body, _ := json.Marshal(resp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	next := c.ListCommentThreads(context.Background(), "tok", "vid")

	first, ok, err := next()
	if err != nil || !ok {
		t.Fatalf("first: ok=%v err=%v", ok, err)
	}
	if first.CommentID != "abc" || first.ParentCommentID != nil || first.AuthorHandle != "Alice" || first.Body != "hi" {
		t.Fatalf("top-level mapping wrong: %+v", first)
	}
	second, ok, err := next()
	if err != nil || !ok {
		t.Fatalf("second: ok=%v err=%v", ok, err)
	}
	if second.CommentID != "xyz" || second.ParentCommentID == nil || *second.ParentCommentID != "abc" || second.AuthorHandle != "Bob" {
		t.Fatalf("reply mapping wrong: %+v", second)
	}
	if _, ok, _ := next(); ok {
		t.Fatal("expected exhaustion")
	}
}

func TestParseRetryAfterAny(t *testing.T) {
	if d := parseRetryAfterAny("60"); d != 60*time.Second {
		t.Fatalf("expected 60s got %v", d)
	}
	if d := parseRetryAfterAny(""); d != 0 {
		t.Fatalf("expected 0 got %v", d)
	}
	future := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	if d := parseRetryAfterAny(future); d <= 0 || d > 3*time.Minute {
		t.Fatalf("expected ~2m got %v", d)
	}
}

func TestDeriveLockID_Deterministic(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	a := DeriveLockID(id)
	b := DeriveLockID(id)
	if a != b {
		t.Fatalf("expected stable id, got %d vs %d", a, b)
	}
	c := DeriveLockID("00000000-0000-0000-0000-000000000002")
	if a == c {
		t.Fatalf("expected different ids for different uuids")
	}
}
