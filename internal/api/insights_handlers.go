// Package api: insights_handlers.go contains the new handlers introduced by
// the insights-api change: the rich /feed and /saved variants, the star/
// unstar mutations, the export lifecycle, and the insights aggregates.
//
// Cross-cutting rules:
//   - Every handler is registered under the auth + email-verification
//     middleware in main.go.
//   - Every handler returns 503 when INSIGHTS_API_ENABLED=false (gated at
//     the route layer via insightsAPIGate).
//   - Access control is enforced at the SQL layer via BuildFeedQuery's
//     mandatory access-control join; handlers never bypass it.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"noumena/internal/config"
	"noumena/internal/db"
	"noumena/internal/export"
)

// InsightsHandler bundles dependencies for the insights-api endpoints. It
// requires a *db.SQLiteQuery (not the DBQuery interface) because several
// new methods (BuildFeedQuery-driven ones, exports CRUD, aggregates) live
// only on the concrete type.
type InsightsHandler struct {
	Queries *db.SQLiteQuery
	Cfg     *config.Config
}

// insightsAPIGate is a middleware that returns 503 when the insights-api
// feature flag is off. Task 12.2.
func InsightsAPIGate(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.InsightsAPIEnabled {
				writeJSONError(w, http.StatusServiceUnavailable, "insights_api_disabled")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeJSONError emits {"error": code} with the given status. Thin wrapper
// around the package-level writeError for consistency with the rest of the
// API; we keep a separate name so insight call sites read as semantic codes
// (e.g. "feed_query_timeout") rather than human messages.
func writeJSONError(w http.ResponseWriter, status int, code string) {
	writeError(w, status, code)
}

// parseFeedFilters parses the query string into FeedFilters. It rejects
// invalid sort values with 400 and clamps limit to FEED_MAX_LIMIT.
func parseFeedFilters(r *http.Request, cfg *config.Config) (db.FeedFilters, error) {
	q := r.URL.Query()
	f := db.FeedFilters{
		Limit: cfg.FeedDefaultLimit,
		Sort:  "newest",
	}
	if v := q.Get("channels"); v != "" {
		f.Channels = splitCSV(v)
	}
	if v := q.Get("categories"); v != "" {
		f.Categories = splitCSV(v)
	}
	if v := q.Get("insight_type"); v != "" {
		f.InsightType = v
	}
	if v := q.Get("date_from"); v != "" {
		t, err := parseDateOrTime(v)
		if err != nil {
			return f, errors.New("invalid date_from")
		}
		f.DateFrom = &t
	}
	if v := q.Get("date_to"); v != "" {
		t, err := parseDateOrTime(v)
		if err != nil {
			return f, errors.New("invalid date_to")
		}
		f.DateTo = &t
	}
	if v := q.Get("search"); v != "" {
		f.Search = &v
	}
	if v := q.Get("include_toxic"); v == "true" || v == "1" {
		f.IncludeToxic = true
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, errors.New("invalid limit")
		}
		if n > cfg.FeedMaxLimit {
			n = cfg.FeedMaxLimit
		}
		if n == 0 {
			n = cfg.FeedDefaultLimit
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, errors.New("invalid offset")
		}
		f.Offset = n
	}
	if v := q.Get("sort"); v != "" {
		if _, ok := db.AllowedSorts[v]; !ok {
			return f, errors.New("invalid sort")
		}
		f.Sort = v
	}
	return f, nil
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseDateOrTime(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", v)
}

// feedResponse is the documented shape for /feed and /saved.
type feedResponse struct {
	Comments []*db.FeedRow `json:"comments"`
	Total    *int64        `json:"total"`
	HasMore  bool          `json:"has_more"`
}

// Feed handles GET /api/comments/feed. Tasks 5.1–5.4.
func (h *InsightsHandler) Feed(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	filters, err := parseFeedFilters(r, h.Cfg)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 5.2: parallel feed + count via goroutines.
	var (
		rows     []*db.FeedRow
		feedErr  error
		total    int64
		countErr error
		wg       sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		rows, feedErr = h.Queries.QueryCommentFeed(r.Context(), userID, filters, h.Cfg.FeedQueryTimeout)
	}()
	go func() {
		defer wg.Done()
		total, countErr = h.Queries.CountCommentFeed(r.Context(), userID, filters, h.Cfg.CountQueryTimeout)
	}()
	wg.Wait()

	if errors.Is(feedErr, db.ErrFeedTimeout) {
		writeJSONError(w, http.StatusRequestTimeout, "feed_query_timeout")
		return
	}
	if feedErr != nil {
		writeJSONError(w, http.StatusInternalServerError, "feed_query_failed")
		return
	}

	resp := feedResponse{Comments: rows}
	if rows == nil {
		resp.Comments = []*db.FeedRow{}
	}
	if countErr == nil {
		resp.Total = &total
		resp.HasMore = int64(filters.Offset+len(rows)) < total
		writeJSON(w, http.StatusOK, resp)
		return
	}
	// 5.3: count failed (timeout or other error) — degrade gracefully:
	// total=null, has_more from the page size.
	resp.Total = nil
	resp.HasMore = len(rows) == filters.Limit
	writeJSON(w, http.StatusOK, resp)
}

// Saved handles GET /api/comments/saved. Task 5.5.
func (h *InsightsHandler) Saved(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	filters, err := parseFeedFilters(r, h.Cfg)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	// /saved overrides sort internally; the parsed filters.Sort is ignored
	// by ModeSaved.
	rows, err := h.Queries.QuerySavedFeed(r.Context(), userID, filters, h.Cfg.FeedQueryTimeout)
	if errors.Is(err, db.ErrFeedTimeout) {
		writeJSONError(w, http.StatusRequestTimeout, "feed_query_timeout")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "feed_query_failed")
		return
	}
	resp := feedResponse{Comments: rows}
	if rows == nil {
		resp.Comments = []*db.FeedRow{}
	}
	resp.HasMore = len(rows) == filters.Limit
	writeJSON(w, http.StatusOK, resp)
}

// Star handles POST /api/comments/{id}/star. Tasks 6.1, 6.3.
func (h *InsightsHandler) Star(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	commentID := chi.URLParam(r, "id")
	if !isUUID(commentID) {
		writeJSONError(w, http.StatusBadRequest, "invalid_comment_id")
		return
	}
	hasAccess, err := h.Queries.StarCommentScoped(r.Context(), userID, commentID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "star_failed")
		return
	}
	if !hasAccess {
		writeJSONError(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"comment_id": commentID, "starred": "true"})
}

// Unstar handles DELETE /api/comments/{id}/star. Task 6.2.
func (h *InsightsHandler) Unstar(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	commentID := chi.URLParam(r, "id")
	if !isUUID(commentID) {
		writeJSONError(w, http.StatusBadRequest, "invalid_comment_id")
		return
	}
	hasAccess, err := h.Queries.UnstarComment(r.Context(), userID, commentID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "unstar_failed")
		return
	}
	if !hasAccess {
		writeJSONError(w, http.StatusNotFound, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isUUID(s string) bool {
	// 8-4-4-4-12 hex layout.
	if len(s) != 36 {
		return false
	}
	return isHex(s[0:8]) && isHex(s[9:13]) && isHex(s[14:18]) && isHex(s[19:23]) && isHex(s[24:36]) &&
		s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-'
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune("0123456789abcdefABCDEF", rune(s[i])) {
			return false
		}
	}
	return true
}

// --- window query parsing ---------------------------------------------------

// insightWindowDays maps ?window= labels to day counts; insightWindowLabels
// is the inverse. Data table in place of per-handler switch statements.
var insightWindowDays = map[string]int{"7d": 7, "30d": 30, "90d": 90}
var insightWindowLabels = map[int]string{7: "7d", 30: "30d", 90: "90d"}

// parseWindowDays resolves the analysis window from ?window=7d|30d|90d, then
// ?days=7|30|90; unknown or missing values fall back to defaultDays.
func parseWindowDays(q url.Values, defaultDays int) int {
	days := defaultDays
	if d, ok := insightWindowDays[q.Get("window")]; ok {
		days = d
	}
	if n, ok := queryDays(q); ok {
		days = n
	}
	return days
}

// queryDays parses ?days=, accepting only the catalogued windows.
func queryDays(q url.Values) (int, bool) {
	n, err := strconv.Atoi(q.Get("days"))
	if err != nil {
		return 0, false
	}
	_, ok := insightWindowLabels[n]
	return n, ok
}

// queryDaysStrict is the 400-on-garbage variant used by Trends/Sentiment:
// missing ?days= yields defaultDays, an invalid value reports bad=true.
func queryDaysStrict(q url.Values, defaultDays int) (days int, bad bool) {
	if q.Get("days") == "" {
		return defaultDays, false
	}
	days, ok := queryDays(q)
	return days, !ok
}

// windowLabel renders a day count as its response label (e.g. "7d").
func windowLabel(days int) string {
	if w, ok := insightWindowLabels[days]; ok {
		return w
	}
	return "30d"
}

// windowStart is the "since" timestamp for a day-count window.
func windowStart(days int) time.Time {
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour)
}

// channelLabel renders a nullable channel id for flat JSON payloads.
func channelLabel(channelID *string) string {
	if channelID != nil {
		return *channelID
	}
	return "all"
}

// exportContentTypes maps an export format to its download Content-Type.
var exportContentTypes = map[string]string{"csv": "text/csv", "json": "application/json"}

func contentTypeFor(format string) string {
	if ct, ok := exportContentTypes[format]; ok {
		return ct
	}
	return "application/octet-stream"
}

// requireAuthUser extracts the caller's user id, writing 401 when absent.
func requireAuthUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return userID, true
}

// --- Insights aggregates ----------------------------------------------------

// ActionableInsights handles GET /api/insights/actionable-insights. Returns
// the dashboard aggregate (window, distribution, top insights, total) in one
// payload matching the frontend contract; the /top and /distribution
// sub-endpoints remain available for lighter consumers. Window may come as
// `window=7d|30d|90d` (frontend) or `days=7|30|90` (clamped like the other
// aggregate endpoints; invalid values fall back to 30).
func (h *InsightsHandler) ActionableInsights(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	channelID, code := h.validateChannelID(r)
	if code != 0 {
		writeJSONError(w, code, "not_tracked")
		return
	}
	days := parseWindowDays(r.URL.Query(), 30)
	agg, err := h.buildAggregate(r.Context(), userID, channelID, days)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "aggregate_failed")
		return
	}
	writeJSON(w, http.StatusOK, agg)
}

// insightsAggregate is the /actionable-insights dashboard payload matching
// the frontend contract.
type insightsAggregate struct {
	Window        string                `json:"window"`
	ChannelID     *string               `json:"channel_id"`
	Distribution  []db.InsightTypeCount `json:"distribution"`
	TopInsights   []db.TopInsight       `json:"top_insights"`
	TotalInsights int64                 `json:"total_insights"`
}

func (h *InsightsHandler) buildAggregate(ctx context.Context, userID string, channelID *string, days int) (insightsAggregate, error) {
	top, dist, err := h.queryTopAndDistribution(ctx, userID, channelID, windowStart(days))
	if err != nil {
		return insightsAggregate{}, err
	}
	return insightsAggregate{Window: windowLabel(days), ChannelID: channelID,
		Distribution: orEmptyDist(dist), TopInsights: orEmptyTop(top), TotalInsights: sumCounts(dist)}, nil
}

func (h *InsightsHandler) queryTopAndDistribution(ctx context.Context, userID string, channelID *string, since time.Time) ([]db.TopInsight, []db.InsightTypeCount, error) {
	top, err := h.Queries.GetTopInsights(ctx, userID, channelID, since, 50)
	if err != nil {
		return nil, nil, err
	}
	dist, err := h.Queries.GetInsightDistribution(ctx, userID, channelID, since)
	return top, dist, err
}

func orEmptyTop(xs []db.TopInsight) []db.TopInsight {
	if xs == nil {
		return []db.TopInsight{}
	}
	return xs
}

func orEmptyDist(xs []db.InsightTypeCount) []db.InsightTypeCount {
	if xs == nil {
		return []db.InsightTypeCount{}
	}
	return xs
}

func sumCounts(dist []db.InsightTypeCount) int64 {
	var total int64
	for _, d := range dist {
		total += d.Count
	}
	return total
}

// TopInsights handles GET /api/insights/actionable-insights/top.
func (h *InsightsHandler) TopInsights(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	channelID, code := h.validateChannelID(r)
	if code != 0 {
		writeJSONError(w, code, "not_tracked")
		return
	}
	days := parseWindowDays(r.URL.Query(), 30)
	insights, err := h.Queries.GetTopInsights(r.Context(), userID, channelID, windowStart(days), 10)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "aggregate_failed")
		return
	}
	// Group by type and take top 3 per type.
	typeGrouped := make(map[string][]db.TopInsight)
	for _, ins := range insights {
		typeGrouped[ins.Type] = append(typeGrouped[ins.Type], ins)
	}
	resp := make(map[string][]db.TopInsight)
	for typ, list := range typeGrouped {
		if len(list) > 3 {
			list = list[:3]
		}
		resp[typ] = list
	}
	writeJSON(w, http.StatusOK, resp)
}

// InsightDistribution handles GET /api/insights/actionable-insights/distribution.
func (h *InsightsHandler) InsightDistribution(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	channelID, code := h.validateChannelID(r)
	if code != 0 {
		writeJSONError(w, code, "not_tracked")
		return
	}
	days := parseWindowDays(r.URL.Query(), 30)
	dist, err := h.Queries.GetInsightDistribution(r.Context(), userID, channelID, windowStart(days))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "aggregate_failed")
		return
	}
	writeJSON(w, http.StatusOK, dist)
}

// RefreshInsightsView handles POST /api/insights/actionable-insights/refresh.
func (h *InsightsHandler) RefreshInsightsView(w http.ResponseWriter, r *http.Request) {
	_, err := h.Queries.DB().ExecContext(r.Context(), "SELECT refresh_actionable_insights_view()")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "refresh_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
}

// ToxicityShield handles GET /api/insights/toxicity-shield. Task 7.4.
// Returns the requested window's toxicity aggregate in the flat shape the
// dashboard shield card renders (window defaults to 7d).
func (h *InsightsHandler) ToxicityShield(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	channelID, code := h.validateChannelID(r)
	if code != 0 {
		writeJSONError(w, code, "not_tracked")
		return
	}
	days := parseWindowDays(r.URL.Query(), 7)
	b, err := h.Queries.GetToxicityShield(r.Context(), userID, channelID, windowStart(days))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "aggregate_failed")
		return
	}
	writeJSON(w, http.StatusOK, shieldResponse{
		Window:           windowLabel(days),
		ChannelID:        channelLabel(channelID),
		FilteredPct:      b.FilteredPct,
		FilteredComments: b.FilteredCount,
		TotalComments:    b.TotalComments,
	})
}

// shieldResponse is the flat payload the dashboard toxicity shield card
// renders.
type shieldResponse struct {
	Window           string  `json:"window"`
	ChannelID        string  `json:"channel_id"`
	FilteredPct      float64 `json:"filtered_pct"`
	FilteredComments int64   `json:"filtered_comments"`
	TotalComments    int64   `json:"total_comments"`
}

type trendsResponse struct {
	ChannelID     string           `json:"channel_id"`
	Days          int              `json:"days"`
	TopTopics     []db.TopicRow    `json:"top_topics"`
	TopCategories []db.CategoryRow `json:"top_categories"`
}

// Trends handles GET /api/insights/trends. Task 7.5.
func (h *InsightsHandler) Trends(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	days, bad := queryDaysStrict(r.URL.Query(), 30)
	if bad {
		writeJSONError(w, http.StatusBadRequest, "invalid_days")
		return
	}
	channelID, code := h.validateChannelID(r)
	if code != 0 {
		writeJSONError(w, code, "not_tracked")
		return
	}
	topics, err := h.Queries.GetTopTopics(r.Context(), userID, channelID, days, 10)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "trends_failed")
		return
	}
	cats, err := h.Queries.GetTopCategories(r.Context(), userID, channelID, days)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "trends_failed")
		return
	}
	if topics == nil {
		topics = []db.TopicRow{}
	}
	if cats == nil {
		cats = []db.CategoryRow{}
	}
	resp := trendsResponse{
		ChannelID:     "all",
		Days:          days,
		TopTopics:     topics,
		TopCategories: cats,
	}
	if channelID != nil {
		resp.ChannelID = *channelID
	}
	writeJSON(w, http.StatusOK, resp)
}

// validateChannelID returns the parsed channel UUID (or nil) and an HTTP
// status code (0 if all is well, 403 if the user does not track it, 400 if
// the UUID is malformed).
func (h *InsightsHandler) validateChannelID(r *http.Request) (*string, int) {
	v := r.URL.Query().Get("channel_id")
	if v == "" || v == "all" {
		return nil, 0
	}
	if !isUUID(v) {
		return nil, http.StatusBadRequest
	}
	userID, _ := r.Context().Value(UserIDKey).(string)
	ok, err := h.Queries.UserTracksChannel(r.Context(), userID, v)
	if err != nil {
		return nil, http.StatusInternalServerError
	}
	if !ok {
		return nil, http.StatusForbidden
	}
	return &v, 0
}

// sentimentResponse is the documented shape for GET /api/insights/sentiment
// (contract with the dashboard SentimentChart).
type sentimentResponse struct {
	Days   int               `json:"days"`
	Series []db.SentimentDay `json:"series"`
}

// Sentiment handles GET /api/insights/sentiment?channel_id=&days=.
// channel_id is optional: when absent, the series aggregates across every
// channel the caller tracks (global trend). When present it must be a
// channel the user tracks (403 otherwise); days is limited to {7,30,90} and
// defaults to 7. Per-day aggregates are computed at the SQL layer joined on
// the caller's channel access, so untracked channels never leak rows.
func (h *InsightsHandler) Sentiment(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	channelID, code := h.validateChannelID(r)
	if code == http.StatusBadRequest {
		writeJSONError(w, http.StatusBadRequest, "invalid_channel_id")
		return
	}
	if code != 0 {
		writeJSONError(w, code, "not_tracked")
		return
	}
	days, bad := queryDaysStrict(r.URL.Query(), 7)
	if bad {
		writeJSONError(w, http.StatusBadRequest, "invalid_days")
		return
	}
	scoped := ""
	if channelID != nil {
		scoped = *channelID
	}
	series, err := h.Queries.GetSentimentSeries(r.Context(), userID, scoped, days)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "sentiment_failed")
		return
	}
	if series == nil {
		series = []db.SentimentDay{}
	}
	writeJSON(w, http.StatusOK, sentimentResponse{Days: days, Series: series})
}

// --- Export lifecycle -------------------------------------------------------

type exportRequest struct {
	Format string `json:"format"`
	// Filter shape mirrors GET /feed query params.
	Channels     []string `json:"channels"`
	Categories   []string `json:"categories"`
	InsightType  string   `json:"insight_type"`
	DateFrom     *string  `json:"date_from"`
	DateTo       *string  `json:"date_to"`
	Search       *string  `json:"search"`
	IncludeToxic bool     `json:"include_toxic"`
	Sort         string   `json:"sort"`
}

func (er exportRequest) toFilters() (db.FeedFilters, error) {
	f := db.FeedFilters{
		Channels:     er.Channels,
		Categories:   er.Categories,
		InsightType:  er.InsightType,
		Search:       er.Search,
		IncludeToxic: er.IncludeToxic,
		Sort:         er.Sort,
	}
	if er.DateFrom != nil {
		t, err := parseDateOrTime(*er.DateFrom)
		if err != nil {
			return f, errors.New("invalid date_from")
		}
		f.DateFrom = &t
	}
	if er.DateTo != nil {
		t, err := parseDateOrTime(*er.DateTo)
		if err != nil {
			return f, errors.New("invalid date_to")
		}
		f.DateTo = &t
	}
	return f, nil
}

// CreateExport handles POST /api/comments/export. Tasks 8.1–8.4.
func (h *InsightsHandler) CreateExport(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	req, filters, ok := decodeExportRequest(w, r)
	if !ok {
		return
	}
	goAsync, ok := h.shouldGoAsync(w, r, userID, filters)
	if !ok {
		return
	}
	exp, ok := h.enqueueExportJob(w, r, userID, req)
	if !ok {
		return
	}
	if goAsync {
		writeJSON(w, http.StatusAccepted, map[string]any{"job_id": exp.ID, "status": "queued"})
		return
	}
	// 8.3: sync path — encode to disk inline using the same routine the
	// worker uses.
	h.writeSyncExport(w, r, userID, req.Format, filters, exp.ID)
}

// decodeExportRequest validates the format and converts the request into
// feed filters.
func decodeExportRequest(w http.ResponseWriter, r *http.Request) (exportRequest, db.FeedFilters, bool) {
	var req exportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_body")
		return req, db.FeedFilters{}, false
	}
	if req.Format != "csv" && req.Format != "json" {
		writeJSONError(w, http.StatusBadRequest, "invalid_format")
		return req, db.FeedFilters{}, false
	}
	filters, err := req.toFilters()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return req, db.FeedFilters{}, false
	}
	return req, filters, true
}

// shouldGoAsync implements 8.2: count with COUNT_QUERY_TIMEOUT; on timeout
// OR a row count above the sync threshold the job goes async. A non-
// timeout count error is a 500.
func (h *InsightsHandler) shouldGoAsync(w http.ResponseWriter, r *http.Request, userID string, filters db.FeedFilters) (async, ok bool) {
	count, cerr := h.Queries.CountCommentFeed(r.Context(), userID, filters, h.Cfg.CountQueryTimeout)
	if errors.Is(cerr, db.ErrCountTimeout) {
		return true, true
	}
	if cerr != nil {
		writeJSONError(w, http.StatusInternalServerError, "count_failed")
		return false, false
	}
	return count > int64(h.Cfg.ExportSyncThreshold), true
}

// enqueueExportJob persists the request body verbatim into the filters
// JSONB column (audit/replay).
func (h *InsightsHandler) enqueueExportJob(w http.ResponseWriter, r *http.Request, userID string, req exportRequest) (*db.Export, bool) {
	rawFilters, _ := json.Marshal(req)
	exp, err := h.Queries.EnqueueExport(r.Context(), userID, req.Format, rawFilters)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "enqueue_failed")
		return nil, false
	}
	return exp, true
}

// writeSyncExport streams the export to disk and reports the result; every
// failure marks the job failed and surfaces a stable error code.
func (h *InsightsHandler) writeSyncExport(w http.ResponseWriter, r *http.Request, userID, format string, filters db.FeedFilters, exportID string) {
	userDir := filepath.Join(h.Cfg.ExportDir, userID)
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		h.failExport(w, exportID, err, "mkdir_failed")
		return
	}
	filePath := filepath.Join(userDir, exportID+"."+format)
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		h.failExport(w, exportID, err, "open_failed")
		return
	}
	defer f.Close()
	rowCount, err := export.StreamExport(r.Context(), h.Queries.ExportReadDB(), userID, filters, format, f)
	if err != nil {
		h.failExport(w, exportID, err, "encode_failed")
		return
	}
	if err := f.Sync(); err != nil {
		h.failExport(w, exportID, err, "fsync_failed")
		return
	}
	h.writeCompletedExport(w, r, exportID, filePath, rowCount)
}

// failExport marks the job failed (outside the request context, which may
// be dead) and writes the stable error code.
func (h *InsightsHandler) failExport(w http.ResponseWriter, exportID string, cause error, code string) {
	_ = h.Queries.MarkExportFailed(context.Background(), exportID, cause.Error(), false)
	writeJSONError(w, http.StatusInternalServerError, code)
}

func (h *InsightsHandler) writeCompletedExport(w http.ResponseWriter, r *http.Request, exportID, filePath string, rowCount int64) {
	if err := h.Queries.MarkExportCompleted(r.Context(), exportID, filePath, rowCount, h.Cfg.ExportTTLHours); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "mark_completed_failed")
		return
	}
	expiresAt := time.Now().Add(time.Duration(h.Cfg.ExportTTLHours) * time.Hour)
	writeJSON(w, http.StatusOK, map[string]any{
		"export_id":    exportID,
		"download_url": "/api/comments/export/" + exportID + "/download",
		"expires_at":   expiresAt.Format(time.RFC3339),
		"row_count":    rowCount,
	})
}

// ListRecentExports handles GET /api/comments/export/recent (fix-frontend-
// plumbing). Returns the caller's most recent comment exports, newest first.
func (h *InsightsHandler) ListRecentExports(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	exports, err := h.Queries.ListExports(r.Context(), userID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list_failed")
		return
	}
	out := make([]map[string]any, 0, len(exports))
	for _, exp := range exports {
		row := map[string]any{
			"job_id":     exp.ID,
			"format":     exp.Format,
			"status":     exp.Status,
			"row_count":  exp.RowCount,
			"created_at": exp.CreatedAt.Format(time.RFC3339),
		}
		if exp.CompletedAt != nil {
			row["completed_at"] = exp.CompletedAt.Format(time.RFC3339)
		}
		if exp.ExpiresAt != nil {
			row["expires_at"] = exp.ExpiresAt.Format(time.RFC3339)
		}
		if exp.Status == "completed" {
			row["download_url"] = "/api/comments/export/" + exp.ID + "/download"
		}
		if exp.Error != nil {
			row["error"] = *exp.Error
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

// ExportStatus handles GET /api/comments/export/{job_id}/status. Task 8.5.
func (h *InsightsHandler) ExportStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	jobID := chi.URLParam(r, "id")
	if !isUUID(jobID) {
		writeJSONError(w, http.StatusBadRequest, "invalid_job_id")
		return
	}
	exp, err := h.Queries.GetExport(r.Context(), userID, jobID)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "status_failed")
		return
	}
	resp := map[string]any{
		"job_id":     exp.ID,
		"status":     exp.Status,
		"format":     exp.Format,
		"row_count":  exp.RowCount,
		"created_at": exp.CreatedAt.Format(time.RFC3339),
	}
	if exp.StartedAt != nil {
		resp["started_at"] = exp.StartedAt.Format(time.RFC3339)
	}
	if exp.CompletedAt != nil {
		resp["completed_at"] = exp.CompletedAt.Format(time.RFC3339)
	}
	if exp.ExpiresAt != nil {
		resp["expires_at"] = exp.ExpiresAt.Format(time.RFC3339)
	}
	if exp.Error != nil {
		resp["error"] = *exp.Error
	}
	if exp.Status == "completed" {
		resp["download_url"] = "/api/comments/export/" + exp.ID + "/download"
	}
	writeJSON(w, http.StatusOK, resp)
}

// ExportDownload handles GET /api/comments/export/{export_id}/download. Task 8.6.
func (h *InsightsHandler) ExportDownload(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	exportID := chi.URLParam(r, "id")
	if !isUUID(exportID) {
		writeJSONError(w, http.StatusBadRequest, "invalid_export_id")
		return
	}
	exp, err := h.Queries.GetExport(r.Context(), userID, exportID)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "download_failed")
		return
	}
	if exp.Status == "expired" || exp.FilePath == nil {
		writeJSONError(w, http.StatusGone, "export_expired")
		return
	}
	f, err := os.Open(*exp.FilePath)
	if err != nil {
		// File missing on disk → treat as expired (410 Gone).
		writeJSONError(w, http.StatusGone, "export_expired")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", contentTypeFor(exp.Format))
	w.Header().Set("Content-Disposition", `attachment; filename="noumena-export-`+exp.ID+`.`+exp.Format+`"`)
	_, _ = io.Copy(w, f)
}

// LatestDigest handles GET /api/insights/digest/latest. Task 9.1, 9.2.
func (h *InsightsHandler) LatestDigest(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuthUser(w, r)
	if !ok {
		return
	}
	d, err := h.Queries.GetLatestDigest(r.Context(), userID)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "no_digest_yet")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "digest_failed")
		return
	}
	// Surface the JSONB content alongside the documented week_start/week_end.
	// The producer (weekly-digest spec) populates content with the
	// top_ideas/top_questions/sentiment_trend/health_score keys; we pass the
	// blob through verbatim so this read endpoint stays stable across
	// producer changes.
	resp := map[string]any{
		"id":         d.ID,
		"week_start": d.WeekStart,
		"week_end":   d.WeekEnd,
	}
	if len(d.Content) > 0 {
		mergeDigestContent(resp, d.Content)
	}
	writeJSON(w, http.StatusOK, resp)
}

// mergeDigestContent folds the top-level content keys into resp without
// overwriting the documented fields; unparseable JSON is passed through as
// a raw string so this read endpoint stays stable across producer changes.
func mergeDigestContent(resp map[string]any, content []byte) {
	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		resp["content"] = string(content)
		return
	}
	for k, v := range parsed {
		if _, exists := resp[k]; !exists {
			resp[k] = v
		}
	}
}
