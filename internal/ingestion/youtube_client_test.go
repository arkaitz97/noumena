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

func TestParseYouTubeChannelRef(t *testing.T) {
	cases := []struct {
		in        string
		handle    string
		channelID string
	}{
		{"@MrBeast", "MrBeast", ""},
		{"https://www.youtube.com/@MrBeast", "MrBeast", ""},
		{"https://www.youtube.com/@MrBeast/videos", "MrBeast", ""},
		{"https://www.youtube.com/channel/UCX6OQ3DkcsbYNE6H8uQQuVA", "", "UCX6OQ3DkcsbYNE6H8uQQuVA"},
		{"UCX6OQ3DkcsbYNE6H8uQQuVA", "", "UCX6OQ3DkcsbYNE6H8uQQuVA"},
		{"  @  spaced  ", "spaced", ""},
		{"", "", ""},
		{"just a name", "", ""},
	}
	for _, c := range cases {
		h, id := ParseYouTubeChannelRef(c.in)
		if h != c.handle || id != c.channelID {
			t.Errorf("ParseYouTubeChannelRef(%q) = handle %q, id %q; want %q, %q", c.in, h, id, c.handle, c.channelID)
		}
	}
}

func TestYouTubeClient_ResolveChannelRefByHandle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("forHandle"); got != "MrBeast" {
			t.Errorf("forHandle = %q, want MrBeast", got)
		}
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Errorf("key = %q, want test-key", got)
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"UCX6OQ3DkcsbYNE6H8uQQuVA","snippet":{"title":"MrBeast"}}]}`))
	}))
	defer srv.Close()
	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL, APIKey: "test-key"}
	info, err := c.ResolveChannelRef(context.Background(), "https://www.youtube.com/@MrBeast/videos")
	if err != nil {
		t.Fatalf("ResolveChannelRef: %v", err)
	}
	if info.ID != "UCX6OQ3DkcsbYNE6H8uQQuVA" || info.DisplayName != "MrBeast" {
		t.Fatalf("got %+v", info)
	}
}

func TestYouTubeClient_ResolveChannelRefByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("id"); got != "UCX6OQ3DkcsbYNE6H8uQQuVA" {
			t.Errorf("id = %q, want UCX6OQ3DkcsbYNE6H8uQQuVA", got)
		}
		_, _ = w.Write([]byte(`{"items":[{"id":"UCX6OQ3DkcsbYNE6H8uQQuVA","snippet":{"title":"MrBeast"}}]}`))
	}))
	defer srv.Close()
	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL, APIKey: "test-key"}
	info, err := c.ResolveChannelRef(context.Background(), "https://www.youtube.com/channel/UCX6OQ3DkcsbYNE6H8uQQuVA")
	if err != nil {
		t.Fatalf("ResolveChannelRef: %v", err)
	}
	if info.ID != "UCX6OQ3DkcsbYNE6H8uQQuVA" {
		t.Fatalf("got %+v", info)
	}
}

func TestYouTubeClient_ResolveChannelRefNoKey(t *testing.T) {
	c := &YouTubeClient{HTTP: http.DefaultClient, BaseURL: "https://example.invalid"}
	if _, err := c.ResolveChannelRef(context.Background(), "@MrBeast"); err == nil {
		t.Fatal("expected error when APIKey unset")
	}
}

func TestYouTubeClient_ResolveChannelRefNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL, APIKey: "test-key"}
	if _, err := c.ResolveChannelRef(context.Background(), "@doesnotexist"); err == nil {
		t.Fatal("expected error when channel not found")
	}
}

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

func TestYouTubeClient_TopLevelCommentDepth(t *testing.T) {
	resp := commentThreadsResponse{
		Items: []commentThreadItem{
			{
				ID: "T1",
				Snippet: struct {
					TopLevelComment ytComment `json:"topLevelComment"`
					TotalReplyCount int       `json:"totalReplyCount"`
				}{
					TopLevelComment: ytComment{
						ID: "top-1",
						Snippet: commentSnippet{
							AuthorDisplayName: "Alice",
							TextDisplay:       "top level",
							PublishedAt:       "2025-01-01T10:00:00Z",
						},
					},
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

	tlc, ok, err := next()
	if err != nil || !ok {
		t.Fatalf("first: ok=%v err=%v", ok, err)
	}
	if tlc.ReplyDepth != 0 {
		t.Fatalf("expected reply_depth=0 for top-level, got %d", tlc.ReplyDepth)
	}
	if tlc.ParentCommentID != nil {
		t.Fatalf("expected parent_comment_id=nil for top-level, got %v", tlc.ParentCommentID)
	}
}

func TestYouTubeClient_ReplyDepthMapping(t *testing.T) {
	tlcID := "top-level-comment"
	resp := commentThreadsResponse{
		Items: []commentThreadItem{
			{
				ID: tlcID,
				Snippet: struct {
					TopLevelComment ytComment `json:"topLevelComment"`
					TotalReplyCount int       `json:"totalReplyCount"`
				}{
					TopLevelComment: ytComment{
						ID: tlcID,
						Snippet: commentSnippet{
							AuthorDisplayName: "Alice",
							TextDisplay:       "top level",
							PublishedAt:       "2025-01-01T10:00:00Z",
						},
					},
				},
				Replies: &struct {
					Comments []ytComment `json:"comments"`
				}{
					Comments: []ytComment{
						{
							ID: "reply-1",
							Snippet: commentSnippet{
								AuthorDisplayName: "Bob",
								TextDisplay:       "reply to top",
								PublishedAt:       "2025-01-01T11:00:00Z",
								ParentID:          tlcID,
							},
						},
						{
							ID: "reply-2",
							Snippet: commentSnippet{
								AuthorDisplayName: "Charlie",
								TextDisplay:       "reply to reply",
								PublishedAt:       "2025-01-01T12:00:00Z",
								ParentID:          "reply-1",
							},
						},
					},
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

	top, _, _ := next()
	if top.ReplyDepth != 0 {
		t.Fatalf("top-level: expected depth=0, got %d", top.ReplyDepth)
	}

	reply1, _, _ := next()
	if reply1.ReplyDepth != 1 {
		t.Fatalf("reply-to-top: expected depth=1, got %d", reply1.ReplyDepth)
	}
	if reply1.ParentCommentID == nil || *reply1.ParentCommentID != tlcID {
		t.Fatalf("reply-to-top: expected parent=%s, got %v", tlcID, reply1.ParentCommentID)
	}

	reply2, _, _ := next()
	if reply2.ReplyDepth != 2 {
		t.Fatalf("reply-to-reply: expected depth=2, got %d", reply2.ReplyDepth)
	}
}

func TestYouTubeClient_ListReplies_Pagination(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			_, _ = w.Write([]byte(`{
				"nextPageToken":"P2",
				"items":[
					{"id":"r1","snippet":{"authorDisplayName":"A","textDisplay":"reply 1","publishedAt":"2025-01-01T10:00:00Z","parentId":"top"}},
					{"id":"r2","snippet":{"authorDisplayName":"B","textDisplay":"reply 2","publishedAt":"2025-01-01T11:00:00Z","parentId":"top"}}
				]}`))
		case 2:
			_, _ = w.Write([]byte(`{
				"items":[
					{"id":"r3","snippet":{"authorDisplayName":"C","textDisplay":"reply 3","publishedAt":"2025-01-01T12:00:00Z","parentId":"top"}}
				]}`))
		}
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	next := c.ListReplies(context.Background(), "tok", "top")

	var ids []string
	for {
		thread, ok, err := next()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !ok {
			break
		}
		ids = append(ids, thread.CommentID)
	}
	if got := len(ids); got != 3 {
		t.Fatalf("expected 3 replies, got %d", got)
	}
	if calls != 2 {
		t.Fatalf("expected 2 API calls, got %d", calls)
	}
}

func TestYouTubeClient_ListReplies_ParentIdExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items":[
				{"id":"r1","snippet":{"authorDisplayName":"A","textDisplay":"reply","publishedAt":"2025-01-01T10:00:00Z","parentId":"parent-123"}}
			]}`))
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	next := c.ListReplies(context.Background(), "tok", "parent-123")

	reply, ok, err := next()
	if err != nil || !ok {
		t.Fatalf("unexpected: ok=%v err=%v", ok, err)
	}
	if reply.ParentCommentID == nil || *reply.ParentCommentID != "parent-123" {
		t.Fatalf("expected parent_id=parent-123, got %v", reply.ParentCommentID)
	}
}

func TestYouTubeClient_ToDBComment_RoundTrip(t *testing.T) {
	parentID := "parent-abc"
	ct := CommentThread{
		CommentID:       "comment-1",
		ParentCommentID: &parentID,
		ReplyDepth:      1,
		AuthorHandle:    "TestUser",
		Body:            "test body",
		PostedAt:        time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
	}

	dbComment := ct.ToDBComment("channel-xyz")

	if dbComment.PlatformCommentID != "comment-1" {
		t.Fatalf("expected platform_comment_id=comment-1, got %s", dbComment.PlatformCommentID)
	}
	if dbComment.ParentCommentID == nil || *dbComment.ParentCommentID != parentID {
		t.Fatalf("expected parent_comment_id=%s, got %v", parentID, dbComment.ParentCommentID)
	}
	if dbComment.ReplyDepth != 1 {
		t.Fatalf("expected reply_depth=1, got %d", dbComment.ReplyDepth)
	}
	if dbComment.AuthorHandle == nil || *dbComment.AuthorHandle != "TestUser" {
		t.Fatalf("expected author_handle=TestUser, got %v", dbComment.AuthorHandle)
	}
	if dbComment.Body != "test body" {
		t.Fatalf("expected body=test body, got %s", dbComment.Body)
	}
}

func TestYouTubeClient_ToDBComment_TopLevel(t *testing.T) {
	ct := CommentThread{
		CommentID:       "top-1",
		ParentCommentID: nil,
		ReplyDepth:      0,
		AuthorHandle:    "TopUser",
		Body:            "top comment",
		PostedAt:        time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
	}

	dbComment := ct.ToDBComment("channel-xyz")

	if dbComment.ParentCommentID != nil {
		t.Fatalf("expected parent_comment_id=nil for top-level, got %v", dbComment.ParentCommentID)
	}
	if dbComment.ReplyDepth != 0 {
		t.Fatalf("expected reply_depth=0 for top-level, got %d", dbComment.ReplyDepth)
	}
}
