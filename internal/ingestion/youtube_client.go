package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"noumena/internal/db"
)

// ============================================================================
// Typed errors surfaced by the YouTube client.
// ============================================================================

// TokenInvalidError indicates the access token has been rejected by YouTube
// (HTTP 401). Callers should fall back to PromoteNextIngestionToken.
type TokenInvalidError struct {
	StatusCode int
	Body       string
}

func (e *TokenInvalidError) Error() string {
	return fmt.Sprintf("youtube token invalid: status=%d", e.StatusCode)
}

// RateLimitError indicates the request was throttled. Callers should park
// the channel for at least RetryAfter duration.
type RateLimitError struct {
	StatusCode int
	RetryAfter time.Duration
	Reason     string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("youtube rate limited: status=%d reason=%s retry_after=%s",
		e.StatusCode, e.Reason, e.RetryAfter)
}

// ============================================================================
// YouTubeClient
// ============================================================================

// YouTubeClient is the typed wrapper around the YouTube Data API v3 calls
// the ingestion worker uses. It owns no state; one client may be shared
// across goroutines. The HTTP client is configurable for tests.
type YouTubeClient struct {
	HTTP    *http.Client
	BaseURL string // override for tests; default https://www.googleapis.com/youtube/v3
}

// NewYouTubeClient returns a client with sensible defaults.
func NewYouTubeClient() *YouTubeClient {
	return &YouTubeClient{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		BaseURL: "https://www.googleapis.com/youtube/v3",
	}
}

func (c *YouTubeClient) base() string {
	if c.BaseURL == "" {
		return "https://www.googleapis.com/youtube/v3"
	}
	return c.BaseURL
}

func (c *YouTubeClient) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// ============================================================================
// CommentThread is the normalized record yielded by ListCommentThreads. Both
// top-level comments and replies are returned as separate CommentThread
// values; replies have ParentCommentID set to the top-level comment id.
// ============================================================================

type CommentThread struct {
	CommentID       string
	ParentCommentID *string
	ReplyDepth      int
	AuthorHandle    string
	Body            string
	PostedAt        time.Time
}

func (t *CommentThread) ToDBComment(platformChannelID string) *db.Comment {
	var author *string
	if t.AuthorHandle != "" {
		a := t.AuthorHandle
		author = &a
	}
	rd := t.ReplyDepth
	return &db.Comment{
		PlatformChannelID: platformChannelID,
		PlatformCommentID: t.CommentID,
		ParentCommentID:   t.ParentCommentID,
		ReplyDepth:        rd,
		AuthorHandle:      author,
		Body:              t.Body,
		PostedAt:          t.PostedAt,
	}
}

// ============================================================================
// search.list — video discovery
// ============================================================================

// VideoRef is the pair (id, publishedAt) yielded by ListVideos.
type VideoRef struct {
	ID          string
	PublishedAt time.Time
}

type searchListResponse struct {
	NextPageToken string `json:"nextPageToken"`
	Items         []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
		Snippet struct {
			PublishedAt string `json:"publishedAt"`
		} `json:"snippet"`
	} `json:"items"`
}

// ListVideos returns a function that paginates search.list, yielding
// (VideoRef, hasMore, err) on each call. When hasMore is false the iteration
// is exhausted. Callers may stop early by simply not calling next() again.
//
// publishedAfter filters server-side; results are returned in descending
// publishedAt (order=date).
func (c *YouTubeClient) ListVideos(ctx context.Context, accessToken, channelID string, publishedAfter time.Time) func() (VideoRef, bool, error) {
	pageToken := ""
	bufferIdx := 0
	var buffer []VideoRef
	exhausted := false

	fetchPage := func() error {
		q := url.Values{}
		q.Set("part", "id,snippet")
		q.Set("channelId", channelID)
		q.Set("type", "video")
		q.Set("order", "date")
		q.Set("maxResults", "50")
		if !publishedAfter.IsZero() {
			q.Set("publishedAfter", publishedAfter.UTC().Format(time.RFC3339))
		}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		body, err := c.doGET(ctx, accessToken, "/search?"+q.Encode())
		if err != nil {
			return err
		}
		var parsed searchListResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return fmt.Errorf("decode search.list: %w", err)
		}
		buffer = buffer[:0]
		for _, it := range parsed.Items {
			if it.ID.VideoID == "" {
				continue
			}
			pubAt, _ := time.Parse(time.RFC3339, it.Snippet.PublishedAt)
			buffer = append(buffer, VideoRef{ID: it.ID.VideoID, PublishedAt: pubAt})
		}
		bufferIdx = 0
		pageToken = parsed.NextPageToken
		if pageToken == "" && len(buffer) == 0 {
			exhausted = true
		}
		return nil
	}

	return func() (VideoRef, bool, error) {
		if exhausted {
			return VideoRef{}, false, nil
		}
		if bufferIdx >= len(buffer) {
			if pageToken == "" && buffer != nil {
				exhausted = true
				return VideoRef{}, false, nil
			}
			if err := fetchPage(); err != nil {
				return VideoRef{}, false, err
			}
			if exhausted {
				return VideoRef{}, false, nil
			}
		}
		ref := buffer[bufferIdx]
		bufferIdx++
		return ref, true, nil
	}
}

// ============================================================================
// commentThreads.list — comment discovery
// ============================================================================

type commentSnippet struct {
	AuthorDisplayName string `json:"authorDisplayName"`
	TextDisplay       string `json:"textDisplay"`
	TextOriginal      string `json:"textOriginal"`
	PublishedAt       string `json:"publishedAt"`
	ParentID          string `json:"parentId"`
}

type ytComment struct {
	ID      string         `json:"id"`
	Snippet commentSnippet `json:"snippet"`
}

type commentThreadItem struct {
	ID      string `json:"id"`
	Snippet struct {
		TopLevelComment ytComment `json:"topLevelComment"`
		TotalReplyCount int       `json:"totalReplyCount"`
	} `json:"snippet"`
	Replies *struct {
		Comments []ytComment `json:"comments"`
	} `json:"replies"`
}

type commentThreadsResponse struct {
	NextPageToken string              `json:"nextPageToken"`
	Items         []commentThreadItem `json:"items"`
}

type commentsListResponse struct {
	NextPageToken string      `json:"nextPageToken"`
	Items         []ytComment `json:"items"`
}

func (c *YouTubeClient) ListReplies(ctx context.Context, accessToken, parentCommentID string) func() (CommentThread, bool, error) {
	pageToken := ""
	var buffer []CommentThread
	idx := 0
	exhausted := false

	fetchPage := func() error {
		q := url.Values{}
		q.Set("part", "snippet")
		q.Set("parentId", parentCommentID)
		q.Set("maxResults", "100")
		q.Set("textFormat", "plainText")
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		body, err := c.doGET(ctx, accessToken, "/comments?"+q.Encode())
		if err != nil {
			return err
		}
		var parsed commentsListResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return fmt.Errorf("decode comments.list (replies): %w", err)
		}
		buffer = buffer[:0]
		for _, c := range parsed.Items {
			if c.ID == "" {
				continue
			}
			rPosted, _ := time.Parse(time.RFC3339, c.Snippet.PublishedAt)
			rBody := c.Snippet.TextDisplay
			if rBody == "" {
				rBody = c.Snippet.TextOriginal
			}
			pID := c.Snippet.ParentID
			depth := 1
			if pID != "" && pID != parentCommentID {
				depth = 2
			}
			buffer = append(buffer, CommentThread{
				CommentID:       c.ID,
				ParentCommentID: &pID,
				ReplyDepth:      depth,
				AuthorHandle:    c.Snippet.AuthorDisplayName,
				Body:            rBody,
				PostedAt:        rPosted,
			})
		}
		idx = 0
		pageToken = parsed.NextPageToken
		if pageToken == "" && len(buffer) == 0 {
			exhausted = true
		}
		return nil
	}

	return func() (CommentThread, bool, error) {
		if exhausted {
			return CommentThread{}, false, nil
		}
		if idx >= len(buffer) {
			if pageToken == "" && buffer != nil {
				exhausted = true
				return CommentThread{}, false, nil
			}
			if err := fetchPage(); err != nil {
				return CommentThread{}, false, err
			}
			if exhausted {
				return CommentThread{}, false, nil
			}
		}
		t := buffer[idx]
		idx++
		return t, true, nil
	}
}

// ListCommentThreads returns an iterator over CommentThread records for a
// video. Both top-level comments and their replies are flattened into a
// single stream; replies carry ParentCommentID = top-level comment id.
func (c *YouTubeClient) ListCommentThreads(ctx context.Context, accessToken, videoID string) func() (CommentThread, bool, error) {
	pageToken := ""
	var pending []CommentThread
	idx := 0
	exhausted := false

	fetchPage := func() error {
		q := url.Values{}
		q.Set("part", "snippet,replies")
		q.Set("videoId", videoID)
		q.Set("maxResults", "100")
		q.Set("order", "time")
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		body, err := c.doGET(ctx, accessToken, "/commentThreads?"+q.Encode())
		if err != nil {
			return err
		}
		var parsed commentThreadsResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return fmt.Errorf("decode commentThreads.list: %w", err)
		}
		pending = pending[:0]
		for _, item := range parsed.Items {
			tlc := item.Snippet.TopLevelComment
			if tlc.ID == "" {
				continue
			}
			topPosted, _ := time.Parse(time.RFC3339, tlc.Snippet.PublishedAt)
			body := tlc.Snippet.TextDisplay
			if body == "" {
				body = tlc.Snippet.TextOriginal
			}
			pending = append(pending, CommentThread{
				CommentID:       tlc.ID,
				ParentCommentID: nil,
				ReplyDepth:      0,
				AuthorHandle:    tlc.Snippet.AuthorDisplayName,
				Body:            body,
				PostedAt:        topPosted,
			})
			if item.Replies != nil {
				for _, reply := range item.Replies.Comments {
					if reply.ID == "" {
						continue
					}
					rPosted, _ := time.Parse(time.RFC3339, reply.Snippet.PublishedAt)
					rBody := reply.Snippet.TextDisplay
					if rBody == "" {
						rBody = reply.Snippet.TextOriginal
					}
					parentID := tlc.ID
					depth := 1
					if reply.Snippet.ParentID != "" && reply.Snippet.ParentID != tlc.ID {
						depth = 2
					}
					pending = append(pending, CommentThread{
						CommentID:       reply.ID,
						ParentCommentID: &parentID,
						ReplyDepth:      depth,
						AuthorHandle:    reply.Snippet.AuthorDisplayName,
						Body:            rBody,
						PostedAt:        rPosted,
					})
				}
			}
		}
		idx = 0
		pageToken = parsed.NextPageToken
		if pageToken == "" && len(pending) == 0 {
			exhausted = true
		}
		return nil
	}

	return func() (CommentThread, bool, error) {
		if exhausted {
			return CommentThread{}, false, nil
		}
		if idx >= len(pending) {
			if pageToken == "" && pending != nil {
				exhausted = true
				return CommentThread{}, false, nil
			}
			if err := fetchPage(); err != nil {
				return CommentThread{}, false, err
			}
			if exhausted {
				return CommentThread{}, false, nil
			}
		}
		t := pending[idx]
		idx++
		return t, true, nil
	}
}

// ============================================================================
// HTTP plumbing — error classification, no internal retries.
// ============================================================================

// doGET issues a GET to baseURL+path with bearer auth and returns the body.
// It surfaces typed errors for 401 and rate-limit conditions; all other
// non-2xx responses become a plain error. There are NO internal retries —
// retry policy is owned by the worker.
func (c *YouTubeClient) doGET(ctx context.Context, accessToken, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body := readAll(resp)

	if resp.StatusCode == http.StatusOK {
		return body, nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &TokenInvalidError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	// Rate-limit detection: 429, OR 403 with reason quotaExceeded /
	// rateLimitExceeded in the JSON error body.
	if resp.StatusCode == http.StatusTooManyRequests {
		ra := parseRetryAfter(resp.Header.Get("Retry-After"))
		if ra <= 0 {
			ra = 1 * time.Hour
		}
		return nil, &RateLimitError{StatusCode: resp.StatusCode, RetryAfter: ra, Reason: "rateLimited"}
	}
	if resp.StatusCode == http.StatusForbidden {
		var parsed struct {
			Error struct {
				Errors []struct {
					Reason string `json:"reason"`
				} `json:"errors"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &parsed)
		for _, e := range parsed.Error.Errors {
			if e.Reason == "quotaExceeded" || e.Reason == "rateLimitExceeded" || e.Reason == "userRateLimitExceeded" {
				ra := parseRetryAfter(resp.Header.Get("Retry-After"))
				if ra <= 0 {
					ra = 1 * time.Hour
				}
				return nil, &RateLimitError{StatusCode: resp.StatusCode, RetryAfter: ra, Reason: e.Reason}
			}
		}
	}

	return nil, fmt.Errorf("youtube %s: HTTP %d: %s", path, resp.StatusCode, truncate(string(body), 256))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// parseRetryAfterAny is a stricter variant of parseRetryAfter that also
// accepts integers > 0 and HTTP-date forms. Kept here for unit tests.
func parseRetryAfterAny(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}
