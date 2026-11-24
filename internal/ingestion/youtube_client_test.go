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
				Snippet: commentThreadSnippet{
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
				Snippet: commentThreadSnippet{
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
				Snippet: commentThreadSnippet{
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

func TestYouTubeClient_ChannelRowsWithoutVideoAreExcluded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[
			{"snippet":{"videoId":"v1","topLevelComment":{"id":"video-comment","snippet":{"publishedAt":"2025-01-01T10:00:00Z","videoId":"v1"}}}},
			{"snippet":{"topLevelComment":{"id":"channel-comment","snippet":{"publishedAt":"2025-01-01T10:00:00Z"}}}}
		]}`))
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	next := c.ListChannelCommentThreadsPaged(context.Background(), "tok", "channel", "", nil)
	thread, ok, err := next()
	if err != nil || !ok || thread.CommentID != "video-comment" {
		t.Fatalf("expected only video-associated row, got %+v ok=%v err=%v", thread, ok, err)
	}
	if _, ok, err := next(); err != nil || ok {
		t.Fatalf("expected channel-only row to be excluded, ok=%v err=%v", ok, err)
	}
}

func TestYouTubeClient_InlineReplyPreservesActualParent(t *testing.T) {
	items := []commentThreadItem{{
		Snippet: commentThreadSnippet{TopLevelComment: ytComment{ID: "top", Snippet: commentSnippet{PublishedAt: "2025-01-01T10:00:00Z", VideoID: "v1"}}},
		Replies: &struct {
			Comments []ytComment `json:"comments"`
		}{Comments: []ytComment{{
			ID: "nested", Snippet: commentSnippet{ParentID: "reply-parent", PublishedAt: "2025-01-01T11:00:00Z", VideoID: "v1"},
		}}},
	}}
	threads := threadsFromItems(items)
	if len(threads) != 2 || threads[1].ParentCommentID == nil || *threads[1].ParentCommentID != "reply-parent" {
		t.Fatalf("inline reply parent was not preserved: %+v", threads)
	}
}

// --- uploads-playlist discovery (optimize-youtube-api-quota) ---------------

func TestYouTubeClient_ResolveUploadsPlaylist(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		_, _ = w.Write([]byte(`{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU123"}}}]}`))
	}))
	defer srv.Close()
	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	id, err := c.ResolveUploadsPlaylist(context.Background(), "tok", "UC1")
	if err != nil {
		t.Fatalf("ResolveUploadsPlaylist: %v", err)
	}
	if id != "UU123" {
		t.Fatalf("got %q, want UU123", id)
	}
	if !strings.Contains(gotPath, "part=contentDetails") || !strings.Contains(gotPath, "fields=") {
		t.Fatalf("missing part/fields projection: %s", gotPath)
	}
}

func TestYouTubeClient_ListUploads_PaginationAndFields(t *testing.T) {
	calls := 0
	var sawFields bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("fields") != "" {
			sawFields = true
		}
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			_, _ = w.Write([]byte(`{
				"nextPageToken":"P2",
				"items":[
					{"contentDetails":{"videoId":"v1"},"snippet":{"publishedAt":"2025-01-03T10:00:00Z","title":"One"}},
					{"contentDetails":{"videoId":"v2"},"snippet":{"publishedAt":"2025-01-02T10:00:00Z","title":"Two"}}
				]}`))
		case 2:
			_, _ = w.Write([]byte(`{
				"items":[
					{"contentDetails":{"videoId":"v3"},"snippet":{"publishedAt":"2025-01-01T10:00:00Z","title":"Three"}}
				]}`))
		}
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	next := c.ListUploads(context.Background(), "tok", "UU1", time.Time{})
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
	if calls != 2 || !sawFields {
		t.Fatalf("calls=%d sawFields=%v", calls, sawFields)
	}
}

func TestYouTubeClient_ListUploads_StopsAtWatermark(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// Newest-first page entirely older than the stop boundary.
		_, _ = w.Write([]byte(`{
			"nextPageToken":"P2",
			"items":[
				{"contentDetails":{"videoId":"v1"},"snippet":{"publishedAt":"2025-01-01T10:00:00Z"}},
				{"contentDetails":{"videoId":"v2"},"snippet":{"publishedAt":"2024-12-31T10:00:00Z"}}
			]}`))
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	stop := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	next := c.ListUploads(context.Background(), "tok", "UU1", stop)
	v, ok, err := next()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok || v.ID != "" {
		t.Fatalf("expected immediate stop at watermark, got %q ok=%v", v.ID, ok)
	}
	if calls != 1 {
		t.Fatalf("expected single page then stop, got %d calls", calls)
	}
}

// --- reply gating metadata (1.5) ---------------------------------------------

func TestYouTubeClient_ThreadReplyCountsPreserved(t *testing.T) {
	resp := commentThreadsResponse{
		Items: []commentThreadItem{
			{
				ID: "T1",
				Snippet: commentThreadSnippet{
					TopLevelComment: ytComment{ID: "top", Snippet: commentSnippet{
						AuthorDisplayName: "A", TextDisplay: "t", PublishedAt: "2025-01-01T10:00:00Z",
					}},
					TotalReplyCount: 5,
				},
				Replies: &struct {
					Comments []ytComment `json:"comments"`
				}{Comments: []ytComment{
					{ID: "r1", Snippet: commentSnippet{TextDisplay: "x", PublishedAt: "2025-01-01T11:00:00Z", ParentID: "top"}},
					{ID: "r2", Snippet: commentSnippet{TextDisplay: "y", PublishedAt: "2025-01-01T12:00:00Z", ParentID: "top"}},
				}},
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
	top, ok, err := next()
	if err != nil || !ok {
		t.Fatalf("first: ok=%v err=%v", ok, err)
	}
	if top.TotalReplyCount != 5 || top.EmbeddedReplyCount != 2 {
		t.Fatalf("gating metadata lost: total=%d embedded=%d", top.TotalReplyCount, top.EmbeddedReplyCount)
	}
}

// --- quota accounting (3.1/3.2/3.3) ------------------------------------------

func TestQuotaMethodUnits(t *testing.T) {
	cases := map[string]int64{
		"/search?key=k":             100,
		"/playlistItems?part=x":     1,
		"/channels?id=UC1":          1,
		"/commentThreads?videoId=v": 1,
		"/comments?parentId=c":      1,
	}
	for path, want := range cases {
		name, units := youtubeMethodAndUnits(path)
		if units != want {
			t.Errorf("units(%s)=%d want %d", path, units, want)
		}
		if name == "" {
			t.Errorf("empty method for %s", path)
		}
	}
}

func TestQuotaAccounting_DailyBudget(t *testing.T) {
	a := NewQuotaAccounting(150)
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if !a.TryReserve(100, now) || !a.TryReserve(1, now) || !a.TryReserve(49, now) {
		t.Fatal("expected 150 units available")
	}
	if a.TryReserve(1, now) {
		t.Fatal("expected budget spent")
	}
	// UTC-day roll resets the window.
	if !a.TryReserve(1, now.Add(13*time.Hour)) {
		t.Fatal("expected new-day budget")
	}
	// Hard stop ignores the window until it passes.
	b := NewQuotaAccounting(0)
	b.MarkExhausted(now)
	if b.TryReserve(1, now) {
		t.Fatal("expected hard stop to block")
	}
	if !b.TryReserve(1, now.Add(24*time.Hour)) {
		t.Fatal("expected hard stop to expire at next UTC day")
	}
}

func TestYouTubeClient_DailyBudgetSuppressesRequests(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL, Quota: NewQuotaAccounting(2)}
	ctx := context.Background()
	// commentThreads = 1 unit each; two fit, the third never leaves the
	// process. search.list would cost 100 and be refused outright.
	for i := 0; i < 2; i++ {
		next := c.ListCommentThreads(ctx, "tok", "v")
		if _, _, err := next(); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	next := c.ListCommentThreads(ctx, "tok", "v")
	_, _, err := next()
	var qe *QuotaExhaustedError
	if !errors.As(err, &qe) {
		t.Fatalf("expected QuotaExhaustedError, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 network calls, got %d", calls)
	}
	if qe.UnitsSpent != 2 {
		t.Fatalf("UnitsSpent=%d want 2", qe.UnitsSpent)
	}
}

func TestYouTubeClient_JobRequestCeiling(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL}
	ctx := WithJobRequestBudget(context.Background(), 1)
	if _, _, err := c.ListCommentThreads(ctx, "tok", "v")(); err != nil {
		t.Fatalf("first request: %v", err)
	}
	_, _, err := c.ListCommentThreads(ctx, "tok", "v2")()
	var jb *JobBudgetExceededError
	if !errors.As(err, &jb) || jb.Kind != "requests" {
		t.Fatalf("expected JobBudgetExceededError(requests), got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 network call, got %d", calls)
	}
}

func TestYouTubeClient_QuotaExceededHardStopsAccounting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"errors":[{"reason":"quotaExceeded"}]}}`))
	}))
	defer srv.Close()

	c := &YouTubeClient{HTTP: srv.Client(), BaseURL: srv.URL, Quota: NewQuotaAccounting(10000)}
	ctx := context.Background()
	next := c.ListCommentThreads(ctx, "tok", "v")
	if _, _, err := next(); err == nil {
		t.Fatal("expected rate-limit error")
	}
	// After a server quotaExceeded, further requests are suppressed without
	// touching the network.
	calls := 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv2.Close()
	c2 := &YouTubeClient{HTTP: srv2.Client(), BaseURL: srv2.URL, Quota: c.Quota}
	next2 := c2.ListCommentThreads(ctx, "tok", "v")
	_, _, err2 := next2()
	var qe *QuotaExhaustedError
	if !errors.As(err2, &qe) {
		t.Fatalf("expected suppression after hard stop, got %v", err2)
	}
	if calls != 0 {
		t.Fatalf("expected no network calls after hard stop, got %d", calls)
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
