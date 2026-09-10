package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
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

// QuotaExhaustedError indicates the request was suppressed locally because
// the project's daily YouTube quota budget (or a recorded quotaExceeded
// hard-stop) is spent. Callers should stop the current job and defer the
// channel WITHOUT consuming retry budget.
type QuotaExhaustedError struct {
	UnitsSpent int64
	DailyLimit int64
	RetryAt    time.Time
}

func (e *QuotaExhaustedError) Error() string {
	return fmt.Sprintf("youtube quota exhausted: spent=%d limit=%d retry_at=%s",
		e.UnitsSpent, e.DailyLimit, e.RetryAt.UTC().Format(time.RFC3339))
}

// JobBudgetExceededError indicates a per-job ceiling was reached (requests,
// videos, pages, or replies). Like quota exhaustion it is a clean stop, not
// a failure.
type JobBudgetExceededError struct {
	Kind  string // requests | videos | pages | replies
	Limit int
}

func (e *JobBudgetExceededError) Error() string {
	return fmt.Sprintf("youtube job %s ceiling reached: limit=%d", e.Kind, e.Limit)
}

// ============================================================================
// Quota accounting — centralized method costs, daily budget, job ceilings.
// ============================================================================

// youtubeMethodUnits centralizes Data API v3 quota costs (units per request)
// for every method the ingestion worker calls. search.list is the 100-unit
// outlier this change exists to avoid.
var youtubeMethodUnits = map[string]int64{
	"search.list":         100,
	"playlistItems.list":  1,
	"channels.list":       1,
	"commentThreads.list": 1,
	"comments.list":       1,
}

// youtubeMethodAndUnits maps a client request path (e.g. "/commentThreads?...")
// to its API method name and quota cost. Unknown paths cost 1 unit.
func youtubeMethodAndUnits(path string) (string, int64) {
	name := path
	if i := strings.IndexAny(name, "?"); i >= 0 {
		name = name[:i]
	}
	name = strings.TrimPrefix(name, "/")
	key := name + ".list"
	if u, ok := youtubeMethodUnits[key]; ok {
		return key, u
	}
	return name, 1
}

// QuotaAccounting tracks consumed units against a daily project budget and
// records hard stops (quotaExceeded responses) until the next UTC day. Safe
// for concurrent use; a shared YouTubeClient may be driven by many workers.
type QuotaAccounting struct {
	mu            sync.Mutex
	day           string // UTC date of the current budget window
	units         int64
	dailyLimit    int64 // 0 = unlimited
	hardStopUntil time.Time
}

// NewQuotaAccounting returns accounting for a daily unit budget. dailyLimit
// of 0 disables budget enforcement (requests are still metered).
func NewQuotaAccounting(dailyLimit int64) *QuotaAccounting {
	return &QuotaAccounting{dailyLimit: dailyLimit, day: utcDay(time.Now())}
}

func utcDay(t time.Time) string { return t.UTC().Format("2006-01-02") }

// nextUTCDay is the UTC-midnight budget reset instant after t.
func nextUTCDay(t time.Time) time.Time {
	return t.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
}

// ytErrorStatus buckets a doGET error into the closed status set recorded
// on noumena_youtube_requests_total{status}.
func ytErrorStatus(err error) string {
	if label, ok := typedErrorStatus(err); ok {
		return label
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "ctx_error"
	}
	return "error"
}

// typedErrorStatus probes the closed typed-error set with errors.As; order
// preserves the original bucketing.
func typedErrorStatus(err error) (string, bool) {
	var tok *TokenInvalidError
	if errors.As(err, &tok) {
		return "unauthorized", true
	}
	var rl *RateLimitError
	if errors.As(err, &rl) {
		return "rate_limited", true
	}
	var cd *CommentsDisabledError
	if errors.As(err, &cd) {
		return "comments_disabled", true
	}
	var qe *QuotaExhaustedError
	if errors.As(err, &qe) {
		return "quota_exhausted", true
	}
	var jb *JobBudgetExceededError
	if errors.As(err, &jb) {
		return "job_ceiling", true
	}
	return "", false
}

// TryReserve consumes units for an outgoing request, returning false when the
// budget is spent or a hard stop is active. A new UTC day rolls the window.
func (a *QuotaAccounting) TryReserve(units int64, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if d := utcDay(now); d != a.day {
		a.day, a.units = d, 0
	}
	if now.Before(a.hardStopUntil) {
		return false
	}
	if a.dailyLimit > 0 && a.units+units > a.dailyLimit {
		return false
	}
	a.units += units
	return true
}

// MarkExhausted records a server-side quotaExceeded rejection and stops
// admission until the next UTC midnight (when Google's daily budget resets).
func (a *QuotaAccounting) MarkExhausted(now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	next := now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	if next.After(a.hardStopUntil) {
		a.hardStopUntil = next
	}
	if a.dailyLimit > 0 {
		a.units = a.dailyLimit
	}
}

// Spend records units for a request the accounting did not admit (e.g. a
// quota rejection that still consumed the unit window).
func (a *QuotaAccounting) Spend(units int64, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if d := utcDay(now); d != a.day {
		a.day, a.units = d, 0
	}
	a.units += units
}

// UnitsSpent returns the units consumed in the current window.
func (a *QuotaAccounting) UnitsSpent() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.units
}

// jobBudget carries a per-job request ceiling through context so the
// stateless, shared client can admit or suppress requests per job without
// global state. Zero remaining ⇒ JobBudgetExceededError.
type jobBudget struct {
	mu    sync.Mutex
	used  int
	limit int
}

type jobBudgetKey struct{}

// WithJobRequestBudget bounds the number of YouTube requests made while ctx
// is used. limit <= 0 means unlimited.
func WithJobRequestBudget(ctx context.Context, limit int) context.Context {
	if limit <= 0 {
		return ctx
	}
	return context.WithValue(ctx, jobBudgetKey{}, &jobBudget{limit: limit})
}

func jobBudgetFrom(ctx context.Context) *jobBudget {
	jb, _ := ctx.Value(jobBudgetKey{}).(*jobBudget)
	return jb
}

func (jb *jobBudget) take() bool {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	if jb.used >= jb.limit {
		return false
	}
	jb.used++
	return true
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
	// Quota, when non-nil, meters every request against the project's daily
	// unit budget and suppresses calls once it is spent.
	Quota *QuotaAccounting
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

	// Reply-gating metadata, set on TOP-LEVEL records only
	// (optimize-youtube-api-quota): callers must fetch deep replies via
	// comments.list only when TotalReplyCount > EmbeddedReplyCount.
	TotalReplyCount    int
	EmbeddedReplyCount int
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

// searchItem is one search.list row.
type searchItem struct {
	ID struct {
		VideoID string `json:"videoId"`
	} `json:"id"`
	Snippet struct {
		PublishedAt string `json:"publishedAt"`
		Title       string `json:"title"`
	} `json:"snippet"`
}

type searchListResponse struct {
	NextPageToken string       `json:"nextPageToken"`
	Items         []searchItem `json:"items"`
}

// ListVideos returns a function that paginates search.list, yielding
// (VideoRef, hasMore, err) on each call. When hasMore is false the iteration
// is exhausted. Callers may stop early by simply not calling next() again.
//
// publishedAfter filters server-side; results are returned in descending
// publishedAt (order=date).
// pageIterator walks a page-buffered list endpoint: load refills the
// buffer and reports whether a next page exists; next serves buffered items
// until the stream is exhausted. The first next() call triggers a fetch.
type pageIterator[T any] struct {
	load    func() ([]T, bool, error)
	items   []T
	idx     int
	hasMore bool
	done    bool
}

func (it *pageIterator[T]) next() (T, bool, error) {
	var zero T
	for !it.done && it.idx >= len(it.items) {
		if !it.hasMore {
			it.done = true
			return zero, false, nil
		}
		items, more, err := it.load()
		if err != nil {
			it.done = true
			return zero, false, err
		}
		it.items, it.idx, it.hasMore = items, 0, more
	}
	if it.idx >= len(it.items) {
		it.done = true
		return zero, false, nil
	}
	item := it.items[it.idx]
	it.idx++
	return item, true, nil
}

// pageFields appends the shared list-pagination params.
func pageFields(q url.Values, maxResults int, fields, pageToken string) {
	q.Set("maxResults", strconv.Itoa(maxResults))
	q.Set("fields", fields)
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
}

func (c *YouTubeClient) ListVideos(ctx context.Context, accessToken, channelID string, publishedAfter time.Time) func() (VideoRef, bool, error) {
	pageToken := ""
	it := &pageIterator[VideoRef]{hasMore: true}
	it.load = func() ([]VideoRef, bool, error) {
		q := url.Values{}
		q.Set("part", "id,snippet")
		q.Set("channelId", channelID)
		q.Set("type", "video")
		q.Set("order", "date")
		pageFields(q, 50, "nextPageToken,items(id/videoId,snippet/publishedAt,snippet/title)", pageToken)
		if !publishedAfter.IsZero() {
			q.Set("publishedAfter", publishedAfter.UTC().Format(time.RFC3339))
		}
		body, err := c.doGET(ctx, accessToken, "/search?"+q.Encode())
		if err != nil {
			return nil, false, err
		}
		var parsed searchListResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, false, fmt.Errorf("decode search.list: %w", err)
		}
		pageToken = parsed.NextPageToken
		return videoRefsFromSearch(parsed.Items), pageToken != "", nil
	}
	return it.next
}

// videoRefsFromSearch decodes search.list items, skipping rows without an id.
func videoRefsFromSearch(items []searchItem) []VideoRef {
	refs := make([]VideoRef, 0, len(items))
	for _, it := range items {
		if it.ID.VideoID == "" {
			continue
		}
		pubAt, _ := time.Parse(time.RFC3339, it.Snippet.PublishedAt)
		refs = append(refs, VideoRef{ID: it.ID.VideoID, Title: it.Snippet.Title, PublishedAt: pubAt})
	}
	return refs
}

// ============================================================================
// playlistItems.list — quota-efficient uploads-playlist discovery
// ============================================================================

// playlistItem is one playlistItems.list row.
type playlistItem struct {
	ContentDetails struct {
		VideoID string `json:"videoId"`
	} `json:"contentDetails"`
	Snippet struct {
		PublishedAt string `json:"publishedAt"`
		Title       string `json:"title"`
	} `json:"snippet"`
}

type playlistItemsResponse struct {
	NextPageToken string         `json:"nextPageToken"`
	Items         []playlistItem `json:"items"`
}

// ResolveUploadsPlaylist returns the channel's uploads playlist id via
// channels.list(part=contentDetails) — 1 unit, resolved once per channel and
// persisted by the worker.
func (c *YouTubeClient) ResolveUploadsPlaylist(ctx context.Context, accessToken, channelID string) (string, error) {
	q := url.Values{}
	q.Set("part", "contentDetails")
	q.Set("id", channelID)
	q.Set("fields", "items/contentDetails/relatedPlaylists/uploads")
	body, err := c.doGET(ctx, accessToken, "/channels?"+q.Encode())
	if err != nil {
		return "", err
	}
	var parsed struct {
		Items []struct {
			ContentDetails struct {
				RelatedPlaylists struct {
					Uploads string `json:"uploads"`
				} `json:"relatedPlaylists"`
			} `json:"contentDetails"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode channels.list (uploads): %w", err)
	}
	if len(parsed.Items) == 0 || parsed.Items[0].ContentDetails.RelatedPlaylists.Uploads == "" {
		return "", fmt.Errorf("channel %s has no uploads playlist", channelID)
	}
	return parsed.Items[0].ContentDetails.RelatedPlaylists.Uploads, nil
}

// ListUploads paginates playlistItems.list for a channel's uploads playlist
// (1 unit/page vs 100 units for search.list). Uploads are newest-first, so
// when stopBefore is non-zero the iteration stops early at the first video
// published before it — the per-sync watermark boundary.
func (c *YouTubeClient) ListUploads(ctx context.Context, accessToken, playlistID string, stopBefore time.Time) func() (VideoRef, bool, error) {
	pageToken := ""
	it := &pageIterator[VideoRef]{hasMore: true}
	it.load = func() ([]VideoRef, bool, error) {
		q := url.Values{}
		q.Set("part", "snippet,contentDetails")
		q.Set("playlistId", playlistID)
		pageFields(q, 50, "nextPageToken,items(contentDetails/videoId,snippet/publishedAt,snippet/title)", pageToken)
		body, err := c.doGET(ctx, accessToken, "/playlistItems?"+q.Encode())
		if err != nil {
			return nil, false, err
		}
		var parsed playlistItemsResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, false, fmt.Errorf("decode playlistItems.list: %w", err)
		}
		pageToken = parsed.NextPageToken
		refs, stopped := refsFromPlaylistItems(parsed.Items, stopBefore)
		return refs, !stopped && pageToken != "", nil
	}
	return it.next
}

// refsFromPlaylistItems decodes one uploads page. Descending publishedAt:
// everything after the first item older than the watermark is already
// ingested (stopped=true).
func refsFromPlaylistItems(items []playlistItem, stopBefore time.Time) ([]VideoRef, bool) {
	refs := make([]VideoRef, 0, len(items))
	for _, it := range items {
		if it.ContentDetails.VideoID == "" {
			continue
		}
		pubAt, _ := time.Parse(time.RFC3339, it.Snippet.PublishedAt)
		if !stopBefore.IsZero() && !pubAt.IsZero() && pubAt.Before(stopBefore) {
			return refs, true
		}
		refs = append(refs, VideoRef{ID: it.ContentDetails.VideoID, Title: it.Snippet.Title, PublishedAt: pubAt})
	}
	return refs, false
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
	it := &pageIterator[CommentThread]{hasMore: true}
	it.load = func() ([]CommentThread, bool, error) {
		q := url.Values{}
		q.Set("part", "snippet")
		q.Set("parentId", parentCommentID)
		q.Set("textFormat", "plainText")
		pageFields(q, 100, "nextPageToken,items(id,snippet(authorDisplayName,textDisplay,textOriginal,publishedAt,parentId,videoId))", pageToken)
		body, err := c.doGET(ctx, accessToken, "/comments?"+q.Encode())
		if err != nil {
			return nil, false, err
		}
		var parsed commentsListResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, false, fmt.Errorf("decode comments.list (replies): %w", err)
		}
		pageToken = parsed.NextPageToken
		return repliesFromComments(parsed.Items, parentCommentID), pageToken != "", nil
	}
	return it.next
}

// repliesFromComments decodes comments.list rows into reply threads; a reply
// whose parentId differs from the top-level comment sits one level deeper.
func repliesFromComments(items []ytComment, parentCommentID string) []CommentThread {
	out := make([]CommentThread, 0, len(items))
	for _, c := range items {
		if c.ID == "" {
			continue
		}
		rPosted, _ := time.Parse(time.RFC3339, c.Snippet.PublishedAt)
		pID := c.Snippet.ParentID
		depth := 1
		if pID != "" && pID != parentCommentID {
			depth = 2
		}
		out = append(out, CommentThread{
			CommentID:       c.ID,
			ParentCommentID: &pID,
			ReplyDepth:      depth,
			AuthorHandle:    c.Snippet.AuthorDisplayName,
			Body:            commentBody(c.Snippet),
			PostedAt:        rPosted,
			VideoID:         c.Snippet.VideoID,
		})
	}
	return out
}

// commentBody prefers the display text, falling back to the original.
func commentBody(s commentSnippet) string {
	if s.TextDisplay != "" {
		return s.TextDisplay
	}
	return s.TextOriginal
}

// ListCommentThreads returns an iterator over CommentThread records for a
// video. Both top-level comments and their replies are flattened into a
// single stream; replies carry ParentCommentID = top-level comment id.
func (c *YouTubeClient) ListCommentThreads(ctx context.Context, accessToken, videoID string) func() (CommentThread, bool, error) {
	return c.ListCommentThreadsPaged(ctx, accessToken, videoID, "", nil)
}

// ListCommentThreadsPaged is ListCommentThreads with incremental-sync
// support (optimize-youtube-api-quota): iteration starts from startToken
// ("" = first page) and afterPage is invoked after every fetched page with
// the token used to fetch that page plus the token that resumes after it,
// letting the worker persist durable checkpoints that survive deadlines and
// restarts (resuming at the current page's token is always safe — the
// unique comment constraints deduplicate the replay).
func (c *YouTubeClient) ListCommentThreadsPaged(ctx context.Context, accessToken, videoID, startToken string, afterPage func(fetchedWith, nextToken string)) func() (CommentThread, bool, error) {
	pageToken := startToken
	it := &pageIterator[CommentThread]{hasMore: true}
	it.load = func() ([]CommentThread, bool, error) {
		fetchedWith := pageToken
		body, err := c.doGET(ctx, accessToken, "/commentThreads?"+commentThreadsQuery(videoID, pageToken).Encode())
		if err != nil {
			return nil, false, err
		}
		var parsed commentThreadsResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, false, fmt.Errorf("decode commentThreads.list: %w", err)
		}
		pageToken = parsed.NextPageToken
		if afterPage != nil {
			afterPage(fetchedWith, pageToken)
		}
		return threadsFromItems(parsed.Items), pageToken != "", nil
	}
	return it.next
}

// commentThreadsQuery is the time-ordered top-level + inline-replies page
// request.
func commentThreadsQuery(videoID, pageToken string) url.Values {
	q := url.Values{}
	q.Set("part", "snippet,replies")
	q.Set("videoId", videoID)
	q.Set("order", "time")
	pageFields(q, 100, "nextPageToken,items(snippet/topLevelComment(id,snippet(authorDisplayName,textDisplay,textOriginal,publishedAt,videoId)),snippet/totalReplyCount,replies/comments(id,snippet(authorDisplayName,textDisplay,textOriginal,publishedAt,parentId,videoId)))", pageToken)
	return q
}

// threadsFromItems flattens a commentThreads.list page into a stream of
// top-level threads with their inline replies.
func threadsFromItems(items []commentThreadItem) []CommentThread {
	pending := make([]CommentThread, 0, len(items))
	for _, item := range items {
		if item.Snippet.TopLevelComment.ID == "" {
			continue
		}
		topIdx := len(pending)
		pending = append(pending, topLevelThread(item))
		pending = appendInlineReplies(pending, item, topIdx)
	}
	return pending
}

// topLevelThread decodes the page's parent comment row.
func topLevelThread(item commentThreadItem) CommentThread {
	tlc := item.Snippet.TopLevelComment
	topPosted, _ := time.Parse(time.RFC3339, tlc.Snippet.PublishedAt)
	return CommentThread{
		CommentID:       tlc.ID,
		ParentCommentID: nil,
		ReplyDepth:      0,
		AuthorHandle:    tlc.Snippet.AuthorDisplayName,
		Body:            commentBody(tlc.Snippet),
		PostedAt:        topPosted,
		VideoID:         tlc.Snippet.VideoID,
		TotalReplyCount: item.Snippet.TotalReplyCount,
	}
}

// appendInlineReplies flattens the embedded replies under the thread at
// topIdx and counts them (the worker gates the deep replies.list walk on
// the difference to TotalReplyCount).
func appendInlineReplies(pending []CommentThread, item commentThreadItem, topIdx int) []CommentThread {
	if item.Replies == nil {
		return pending
	}
	parentID := item.Snippet.TopLevelComment.ID
	for _, reply := range item.Replies.Comments {
		if reply.ID == "" {
			continue
		}
		rPosted, _ := time.Parse(time.RFC3339, reply.Snippet.PublishedAt)
		depth := 1
		if reply.Snippet.ParentID != "" && reply.Snippet.ParentID != parentID {
			depth = 2
		}
		pid := parentID
		pending = append(pending, CommentThread{
			CommentID:       reply.ID,
			ParentCommentID: &pid,
			ReplyDepth:      depth,
			AuthorHandle:    reply.Snippet.AuthorDisplayName,
			Body:            commentBody(reply.Snippet),
			PostedAt:        rPosted,
			VideoID:         reply.Snippet.VideoID,
		})
		pending[topIdx].EmbeddedReplyCount++
	}
	return pending
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

// setChannelQuery looks up a channel by id when known, else by @handle.
func setChannelQuery(q url.Values, channelID, handle string) {
	if channelID != "" {
		q.Set("id", channelID)
		return
	}
	q.Set("forHandle", handle)
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
	q.Set("fields", "items(id,snippet/title)")
	setChannelQuery(q, channelID, handle)
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
//
// Quota admission (optimize-youtube-api-quota): before hitting the network
// the request must pass the per-job ceiling (context jobBudget) and the
// daily project budget (c.Quota). Suppressed requests never leave the
// process and are recorded on noumena_youtube_requests_suppressed_total.
func (c *YouTubeClient) doGET(ctx context.Context, accessToken, path string) ([]byte, error) {
	method, units := youtubeMethodAndUnits(path)
	if jb := jobBudgetFrom(ctx); jb != nil && !jb.take() {
		recordYoutubeSuppressed(ctx, "job_ceiling")
		return nil, &JobBudgetExceededError{Kind: "requests", Limit: jb.limit}
	}
	if c.Quota != nil && !c.Quota.TryReserve(units, time.Now()) {
		recordYoutubeSuppressed(ctx, "daily_budget")
		return nil, &QuotaExhaustedError{
			UnitsSpent: c.Quota.UnitsSpent(),
			RetryAt:    nextUTCDay(time.Now()),
		}
	}
	body, err := c.doGETUnmetered(ctx, accessToken, path)
	status := "ok"
	if err != nil {
		status = ytErrorStatus(err)
		// A server-side quotaExceeded means the whole project budget is
		// gone: hard-stop admission until the next UTC day.
		var rl *RateLimitError
		if errors.As(err, &rl) && rl.Reason == "quotaExceeded" && c.Quota != nil {
			c.Quota.MarkExhausted(time.Now())
		}
	}
	recordYoutubeRequest(ctx, method, status, units)
	return body, err
}

// doGETUnmetered is the raw HTTP layer, split out so doGET can account for
// every request exactly once.
func (c *YouTubeClient) doGETUnmetered(ctx context.Context, accessToken, path string) ([]byte, error) {
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

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
