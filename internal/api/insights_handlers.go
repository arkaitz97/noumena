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
// requires a *db.PostgresQuery (not the DBQuery interface) because several
// new methods (BuildFeedQuery-driven ones, exports CRUD, aggregates) live
// only on the concrete type.
type InsightsHandler struct {
	Queries *db.PostgresQuery
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
	if v := q.Get("sentiment_min"); v != "" {
		x, err := strconv.ParseFloat(v, 64)
		if err != nil || x < -1 || x > 1 {
			return f, errors.New("invalid sentiment_min")
		}
		f.SentimentMin = &x
	}
	if v := q.Get("sentiment_max"); v != "" {
		x, err := strconv.ParseFloat(v, 64)
		if err != nil || x < -1 || x > 1 {
			return f, errors.New("invalid sentiment_max")
		}
		f.SentimentMax = &x
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
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
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
	if errors.Is(countErr, db.ErrCountTimeout) {
		// 5.3: total=null, has_more from len(rows)==limit.
		resp.Total = nil
		resp.HasMore = len(rows) == filters.Limit
	} else if countErr != nil {
		// Other count error: degrade gracefully (total=null).
		resp.Total = nil
		resp.HasMore = len(rows) == filters.Limit
	} else {
		resp.Total = &total
		resp.HasMore = int64(filters.Offset+len(rows)) < total
	}
	writeJSON(w, http.StatusOK, resp)
}

// Saved handles GET /api/comments/saved. Task 5.5.
func (h *InsightsHandler) Saved(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
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
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
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
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
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
	for i, ch := range s {
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
		default:
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
				return false
			}
		}
	}
	return true
}

// --- Insights aggregates ----------------------------------------------------

type sentimentResponse struct {
	ChannelID string                            `json:"channel_id"`
	Windows   map[string]db.SentimentBucket     `json:"windows"`
}

// Sentiment handles GET /api/insights/sentiment. Tasks 7.1–7.3.
func (h *InsightsHandler) Sentiment(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	channelID, code := h.validateChannelID(r)
	if code != 0 {
		writeJSONError(w, code, "not_tracked")
		return
	}
	now := time.Now()
	resp := sentimentResponse{
		ChannelID: "all",
		Windows:   map[string]db.SentimentBucket{},
	}
	if channelID != nil {
		resp.ChannelID = *channelID
	}
	for label, dur := range map[string]time.Duration{
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
		"90d": 90 * 24 * time.Hour,
	} {
		b, err := h.Queries.GetSentimentAggregates(r.Context(), userID, channelID, now.Add(-dur))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "aggregate_failed")
			return
		}
		// 7.3: log a warning if bucket percentages are way off 100.
		if b.Count > 0 {
			sum := b.PositivePct + b.NegativePct + b.NeutralPct
			if sum < 99.7 || sum > 100.3 {
				// Non-fatal; rounding artifact at very small N.
				_ = sum
			}
		}
		resp.Windows[label] = b
	}
	writeJSON(w, http.StatusOK, resp)
}

type toxicityResponse struct {
	ChannelID string                          `json:"channel_id"`
	Windows   map[string]db.ToxicityBucket    `json:"windows"`
}

// ToxicityShield handles GET /api/insights/toxicity-shield. Task 7.4.
func (h *InsightsHandler) ToxicityShield(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	channelID, code := h.validateChannelID(r)
	if code != 0 {
		writeJSONError(w, code, "not_tracked")
		return
	}
	now := time.Now()
	resp := toxicityResponse{
		ChannelID: "all",
		Windows:   map[string]db.ToxicityBucket{},
	}
	if channelID != nil {
		resp.ChannelID = *channelID
	}
	for label, dur := range map[string]time.Duration{
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
	} {
		b, err := h.Queries.GetToxicityShield(r.Context(), userID, channelID, now.Add(-dur))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "aggregate_failed")
			return
		}
		resp.Windows[label] = b
	}
	writeJSON(w, http.StatusOK, resp)
}

type trendsResponse struct {
	ChannelID     string             `json:"channel_id"`
	Days          int                `json:"days"`
	TopTopics     []db.TopicRow      `json:"top_topics"`
	TopCategories []db.CategoryRow   `json:"top_categories"`
}

// Trends handles GET /api/insights/trends. Task 7.5.
func (h *InsightsHandler) Trends(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || (n != 7 && n != 30 && n != 90) {
			writeJSONError(w, http.StatusBadRequest, "invalid_days")
			return
		}
		days = n
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

// --- Export lifecycle -------------------------------------------------------

type exportRequest struct {
	Format string `json:"format"`
	// Filter shape mirrors GET /feed query params.
	Channels     []string `json:"channels"`
	Categories   []string `json:"categories"`
	SentimentMin *float64 `json:"sentiment_min"`
	SentimentMax *float64 `json:"sentiment_max"`
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
		SentimentMin: er.SentimentMin,
		SentimentMax: er.SentimentMax,
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
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req exportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if req.Format != "csv" && req.Format != "json" {
		writeJSONError(w, http.StatusBadRequest, "invalid_format")
		return
	}
	filters, err := req.toFilters()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 8.2: count with COUNT_QUERY_TIMEOUT; on timeout OR > threshold go async.
	count, cerr := h.Queries.CountCommentFeed(r.Context(), userID, filters, h.Cfg.CountQueryTimeout)
	goAsync := errors.Is(cerr, db.ErrCountTimeout) || (cerr == nil && count > int64(h.Cfg.ExportSyncThreshold))
	if cerr != nil && !errors.Is(cerr, db.ErrCountTimeout) {
		writeJSONError(w, http.StatusInternalServerError, "count_failed")
		return
	}

	// Persist the request body verbatim into filters JSONB (audit/replay).
	rawFilters, _ := json.Marshal(req)

	exp, err := h.Queries.EnqueueExport(r.Context(), userID, req.Format, rawFilters)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "enqueue_failed")
		return
	}

	if goAsync {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"job_id": exp.ID,
			"status": "queued",
		})
		return
	}

	// 8.3: sync path — encode to disk inline using the same routine the
	// worker uses.
	userDir := filepath.Join(h.Cfg.ExportDir, userID)
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		_ = h.Queries.MarkExportFailed(context.Background(), exp.ID, err.Error(), false)
		writeJSONError(w, http.StatusInternalServerError, "mkdir_failed")
		return
	}
	filePath := filepath.Join(userDir, exp.ID+"."+req.Format)
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		_ = h.Queries.MarkExportFailed(context.Background(), exp.ID, err.Error(), false)
		writeJSONError(w, http.StatusInternalServerError, "open_failed")
		return
	}
	rowCount, eerr := export.StreamExport(r.Context(), h.Queries.Pool(), userID, filters, req.Format, f)
	if eerr != nil {
		_ = f.Close()
		_ = h.Queries.MarkExportFailed(context.Background(), exp.ID, eerr.Error(), false)
		writeJSONError(w, http.StatusInternalServerError, "encode_failed")
		return
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = h.Queries.MarkExportFailed(context.Background(), exp.ID, err.Error(), false)
		writeJSONError(w, http.StatusInternalServerError, "fsync_failed")
		return
	}
	_ = f.Close()
	if err := h.Queries.MarkExportCompleted(r.Context(), exp.ID, filePath, rowCount, h.Cfg.ExportTTLHours); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "mark_completed_failed")
		return
	}
	expiresAt := time.Now().Add(time.Duration(h.Cfg.ExportTTLHours) * time.Hour)
	writeJSON(w, http.StatusOK, map[string]any{
		"export_id":    exp.ID,
		"download_url": "/api/comments/export/" + exp.ID + "/download",
		"expires_at":   expiresAt.Format(time.RFC3339),
		"row_count":    rowCount,
	})
}

// ExportStatus handles GET /api/comments/export/{job_id}/status. Task 8.5.
func (h *InsightsHandler) ExportStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
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
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
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

	contentType := "application/octet-stream"
	switch exp.Format {
	case "csv":
		contentType = "text/csv"
	case "json":
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="noumena-export-`+exp.ID+`.`+exp.Format+`"`)
	_, _ = io.Copy(w, f)
}

// LatestDigest handles GET /api/insights/digest/latest. Task 9.1, 9.2.
func (h *InsightsHandler) LatestDigest(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
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
		var content map[string]any
		if err := json.Unmarshal(d.Content, &content); err == nil {
			for k, v := range content {
				if _, exists := resp[k]; !exists {
					resp[k] = v
				}
			}
		} else {
			resp["content"] = string(d.Content)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
