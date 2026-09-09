package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

// CommentsDisabledError indicates the target video has comments disabled
// (HTTP 403, reason=commentsDisabled). Permanent per-video condition: callers
// should skip the video, not retry.
type CommentsDisabledError struct {
	Path string
}

func (e *CommentsDisabledError) Error() string {
	return fmt.Sprintf("youtube comments disabled: %s", e.Path)
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
	// APIKey, when non-empty, authenticates via ?key= instead of the
	// Authorization bearer header. Enables OAuth-free reads of public data
	// (search/commentThreads/comments). OAuth-only endpoints (channels?mine)
	// still require a token.
	APIKey string
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
	VideoID         string
	VideoTitle      string
}

func (t *CommentThread) ToDBComment(platformChannelID string) *db.Comment {
	var author *string
	if t.AuthorHandle != "" {
		a := t.AuthorHandle
		author = &a
	}
	var videoID, videoTitle *string
	if t.VideoID != "" {
		v := t.VideoID
		videoID = &v
	}
	if t.VideoTitle != "" {
		v := t.VideoTitle
		videoTitle = &v
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
		VideoID:           videoID,
		VideoTitle:        videoTitle,
	}
}

// ============================================================================
// search.list — video discovery
// ============================================================================

// VideoRef is the pair (id, publishedAt) yielded by ListVideos.
type VideoRef struct {
	ID          string
	Title       string
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
			Title       string `json:"title"`
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
			buffer = append(buffer, VideoRef{ID: it.ID.VideoID, Title: it.Snippet.Title, PublishedAt: pubAt})
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
	VideoID           string `json:"videoId"`
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
				VideoID:         c.Snippet.VideoID,
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
				VideoID:         tlc.Snippet.VideoID,
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
						VideoID:         reply.Snippet.VideoID,
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
// Channel resolution (OAuth-free, public)
// ============================================================================

// ParseYouTubeChannelRef extracts a @handle or a bare channel id from raw
// user input. Accepts: `@handle`, `https://www.youtube.com/@handle[/...]`,
// `https://www.youtube.com/channel/UC...`, or a bare `UC...` id. Legacy
// /c/ and /user/ URLs are not supported (YouTube deprecated forUsername).
func ParseYouTubeChannelRef(ref string) (handle, channelID string) {
	s := strings.TrimSpace(ref)
	if s == "" {
		return "", ""
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		s = u.Path
	}
	s = strings.Trim(s, "/")
	if strings.HasPrefix(s, "channel/") {
		id := strings.TrimPrefix(s, "channel/")
		if j := strings.IndexAny(id, "/?#"); j >= 0 {
			id = id[:j]
		}
		return "", id
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		h := strings.TrimSpace(s[i+1:])
		if j := strings.IndexAny(h, "/?#"); j >= 0 {
			h = strings.TrimSpace(h[:j])
		}
		return h, ""
	}
	if strings.HasPrefix(s, "UC") && len(s) >= 24 {
		return "", s
	}
	return "", ""
}

// ResolveChannelRef resolves a public channel from a handle/URL/bare-id ref
// via `channels.list` authenticated with the API key (OAuth-free). Returns
// canonical channel id + display name. Requires APIKey to be set.
func (c *YouTubeClient) ResolveChannelRef(ctx context.Context, ref string) (*ChannelInfo, error) {
	handle, channelID := ParseYouTubeChannelRef(ref)
	if handle == "" && channelID == "" {
		return nil, fmt.Errorf("unsupported channel reference %q: use a @handle or channel URL", ref)
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("public channel tracking not configured on this server (YOUTUBE_API_KEY)")
	}
	q := url.Values{}
	q.Set("part", "snippet")
	if channelID != "" {
		q.Set("id", channelID)
	} else {
		q.Set("forHandle", handle)
	}
	body, err := c.doGET(ctx, "", "/channels?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode channels.list: %w", err)
	}
	if len(parsed.Items) == 0 {
		return nil, fmt.Errorf("no youtube channel found for %q", ref)
	}
	item := parsed.Items[0]
	return &ChannelInfo{ID: item.ID, DisplayName: item.Snippet.Title, Title: item.Snippet.Title}, nil
}

// ============================================================================
// HTTP plumbing — error classification, no internal retries.
// ============================================================================

// doGET issues a GET to baseURL+path with either bearer auth or an API key
// and returns the body. It surfaces typed errors for 401 and rate-limit
// conditions; all other non-2xx responses become a plain error. There are NO
// internal retries — retry policy is owned by the worker.
func (c *YouTubeClient) doGET(ctx context.Context, accessToken, path string) ([]byte, error) {
	if c.APIKey != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		path += sep + "key=" + url.QueryEscape(c.APIKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path, nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey == "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
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
			if e.Reason == "commentsDisabled" {
				return nil, &CommentsDisabledError{Path: path}
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
