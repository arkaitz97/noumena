// Package db: comments_feed.go is the single dynamic SQL builder used by
// /api/comments/feed, /api/comments/saved, the export count, and the export
// streaming worker. Every read path that surfaces comments to a user MUST go
// through BuildFeedQuery so the access-control join (user_channel_tracking)
// and the processed_at predicate cannot be forgotten by a caller.
//
// All filter values are passed as positional ($1, $2, …) parameters; no user
// input is ever interpolated into the SQL string. See BuildFeedQuery_NoSQLi
// in comments_feed_test.go.
package db

import (
	"strings"
	"time"
)

// FeedMode selects the projection and join shape produced by BuildFeedQuery.
type FeedMode int

const (
	// ModeFeed: full row + channel_name + EXISTS(user_comment_stars).
	ModeFeed FeedMode = iota
	// ModeSaved: like ModeFeed but inner-joined to user_comment_stars; sort
	// is forced to starred_at DESC.
	ModeSaved
	// ModeCount: SELECT COUNT(*) only (no ORDER BY/LIMIT/OFFSET).
	ModeCount
	// ModeStream: same projection as ModeFeed, used by the export worker;
	// LIMIT/OFFSET are omitted so the cursor walks the entire result set.
	ModeStream
)

// FeedFilters models every query-string parameter accepted by /feed and
// /saved (with mode-specific overrides applied by the builder).
type FeedFilters struct {
	Channels      []string // platform_channels.id (UUID strings)
	Categories    []string // category enum values
	InsightType   string   // filter by insight type (JSONB containment)
	DateFrom      *time.Time
	DateTo        *time.Time
	Search        *string
	IncludeToxic  bool
	Limit         int
	Offset        int
	Sort          string // "newest" | "oldest"
}

// HighSignalCategories is the default category set applied when the caller
// does not pass a `categories` filter. See specs/comment-feed-api.
var HighSignalCategories = []string{
	"question", "content_idea", "collaboration_ask", "praise", "constructive_criticism",
}

// ToxicCategories are the two categories gated behind the explicit two-layer
// opt-in (Decision 7 in design.md).
var ToxicCategories = map[string]struct{}{
	"toxic_criticism": {},
	"spam":            {},
}

// AllowedSorts enumerates the sort modes accepted on /feed. /saved overrides
// the sort internally and so is not constrained by this set.
var AllowedSorts = map[string]string{
	"newest":  "ORDER BY c.posted_at DESC",
	"oldest":  "ORDER BY c.posted_at ASC",
}

// applyToxicLayer enforces the two-layer toxic filter described in
// specs/comment-feed-api: when categories are explicitly listed, toxic/spam
// pass only if IncludeToxic is also true; otherwise they are stripped.
// Returns the effective category list (may be empty, signalling "default
// high-signal set"), and a boolean indicating whether the caller specified
// any explicit categories at all.
func applyToxicLayer(categories []string, includeToxic bool) ([]string, bool) {
	if len(categories) == 0 {
		// Empty input → caller wants the default high-signal set, regardless
		// of include_toxic. The flag is inert without an explicit category
		// list (Decision 7).
		return nil, false
	}
	out := make([]string, 0, len(categories))
	for _, cat := range categories {
		if _, isToxic := ToxicCategories[cat]; isToxic {
			if includeToxic {
				out = append(out, cat)
			}
			// otherwise silently strip
			continue
		}
		out = append(out, cat)
	}
	return out, true
}

// BuildFeedQuery is the single dynamic SQL builder. It returns the SQL string
// and the positional argument slice. $1 is always userID. The caller MUST NOT
// reorder or omit the access-control join or processed_at predicate; they are
// emitted unconditionally for every mode.
func BuildFeedQuery(userID string, filters FeedFilters, mode FeedMode) (string, []any) {
	args := []any{userID}
	// Helper that records an arg and returns its placeholder ($N).
	addArg := func(v any) string {
		args = append(args, v)
		return "?"
	}

	// --- Projection -------------------------------------------------------
	var projection string
	switch mode {
	case ModeCount:
		projection = "COUNT(*)"
	case ModeFeed, ModeSaved, ModeStream:
		projection = `c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.reply_depth,
			c.author_handle, c.body, c.posted_at, c.category, c.actionable_insights,
			c.topic, c.confidence, c.processed_at, c.created_at,
			pc.display_name AS channel_name,
			EXISTS (SELECT 1 FROM user_comment_stars ucs
			        WHERE ucs.user_id = $1 AND ucs.comment_id = c.id) AS is_starred`
	}

	// --- FROM + JOIN ------------------------------------------------------
	// Access-control join is mandatory and cannot be disabled. We use
	// tracking_status IN ('active','paused') so paused channels remain
	// readable (consistent with the source spec's "still see your data").
	from := `comments c
		JOIN platform_channels pc ON pc.id = c.platform_channel_id
		JOIN user_channel_tracking uct ON uct.platform_channel_id = pc.id
		   AND uct.user_id = $1
		   AND uct.tracking_status IN ('active','paused')`
	if mode == ModeSaved {
		// Inner-join through user_comment_stars to scope the result set to
		// only rows the caller has starred.
		from += `
		JOIN user_comment_stars ucs2 ON ucs2.comment_id = c.id
		   AND ucs2.user_id = $1`
	}

	// --- WHERE ------------------------------------------------------------
	where := []string{"c.processed_at IS NOT NULL"}

	// Effective category list after the two-layer toxic rule.
	effectiveCats, explicitCats := applyToxicLayer(filters.Categories, filters.IncludeToxic)

	if !explicitCats {
		// Default: restrict to the high-signal category set.
		ph := make([]string, 0, len(HighSignalCategories))
		for _, cat := range HighSignalCategories {
			ph = append(ph, addArg(cat))
		}
		where = append(where, "c.category IN ("+strings.Join(ph, ",")+")")
	} else if len(effectiveCats) == 0 {
		// Caller passed only toxic/spam without include_toxic=true → strip
		// reduces to empty list. Return a contradiction so we yield zero
		// rows without dropping the explicit-filter intent.
		where = append(where, "FALSE")
	} else {
		ph := make([]string, 0, len(effectiveCats))
		for _, cat := range effectiveCats {
			ph = append(ph, addArg(cat))
		}
		where = append(where, "c.category IN ("+strings.Join(ph, ",")+")")
	}

	if len(filters.Channels) > 0 {
		ph := make([]string, 0, len(filters.Channels))
		for _, ch := range filters.Channels {
			ph = append(ph, addArg(ch))
		}
		where = append(where, "c.platform_channel_id IN ("+strings.Join(ph, ",")+")")
	}

	if filters.InsightType != "" {
		where = append(where, "json_extract(c.actionable_insights, '$[0].type') = "+addArg(filters.InsightType))
	}
	if filters.DateFrom != nil {
		where = append(where, "c.posted_at >= "+addArg(*filters.DateFrom))
	}
	if filters.DateTo != nil {
		where = append(where, "c.posted_at <= "+addArg(*filters.DateTo))
	}
	if filters.Search != nil && *filters.Search != "" {
		ph := addArg(*filters.Search)
		where = append(where,
			"(c.body LIKE '%' || "+ph+" || '%' OR COALESCE(c.topic,'') LIKE '%' || "+ph+" || '%')")
	}

	// --- ORDER BY / LIMIT / OFFSET ---------------------------------------
	var tail string
	switch mode {
	case ModeCount:
		// no tail
	case ModeSaved:
		// sort overridden per spec
		tail = "ORDER BY ucs2.starred_at DESC"
		tail += " LIMIT " + addArg(safeLimit(filters.Limit))
		tail += " OFFSET " + addArg(safeOffset(filters.Offset))
	case ModeFeed:
		sort, ok := AllowedSorts[filters.Sort]
		if !ok {
			sort = AllowedSorts["newest"]
		}
		tail = sort
		tail += " LIMIT " + addArg(safeLimit(filters.Limit))
		tail += " OFFSET " + addArg(safeOffset(filters.Offset))
	case ModeStream:
		// Export streaming: no LIMIT, deterministic order.
		sort, ok := AllowedSorts[filters.Sort]
		if !ok {
			sort = AllowedSorts["newest"]
		}
		tail = sort
	}

	sql := "SELECT " + projection + " FROM " + from
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	if tail != "" {
		sql += " " + tail
	}
	return sql, args
}

func safeLimit(n int) int {
	if n <= 0 {
		return 20
	}
	if n > 100 {
		return 100
	}
	return n
}

func safeOffset(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
