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
	Channels            []string // platform_channels.id (UUID strings)
	Categories          []string // category enum values
	CategoriesSpecified bool
	InsightType         string // filter by insight type (JSONB containment)
	Intent              string // all | ideas | actionable
	DateFrom            *time.Time
	DateTo              *time.Time
	Search              *string
	IncludeToxic        bool
	Limit               int
	Offset              int
	Sort                string // "newest" | "oldest"
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
	"newest": "ORDER BY c.posted_at DESC, c.id DESC",
	"oldest": "ORDER BY c.posted_at ASC, c.id ASC",
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
// accessControlClause is the mandatory per-user visibility predicate. It is
// kept as a constant so no caller or future refactor can silently drop it.
const accessControlClause = "(EXISTS(SELECT 1 FROM user_channel_tracking uct WHERE uct.platform_channel_id=pc.id AND uct.user_id=$1 AND uct.tracking_status IN ('active','paused')) OR EXISTS(SELECT 1 FROM channel_grants cg JOIN workspace_members wm ON wm.workspace_id=cg.workspace_id WHERE cg.channel_id=pc.id AND wm.user_id=$1))"

// feedProjection is the full comment row with channel name and star flag,
// shared by the feed, saved, and stream modes.
const feedProjection = `c.id, c.platform_channel_id, c.platform_comment_id, c.parent_comment_id, c.reply_depth,
		c.author_handle, c.body, c.posted_at, c.category, c.actionable_insights,
		c.topic, c.confidence, c.video_id, c.video_title, c.processed_at, c.created_at,
		pc.display_name AS channel_name,
		pc.platform_channel_id AS platform_id,
		EXISTS (SELECT 1 FROM user_comment_stars ucs
		        WHERE ucs.user_id = $1 AND ucs.comment_id = c.id) AS is_starred`

// modeProjections maps each feed mode to its SELECT projection clause.
var modeProjections = map[FeedMode]string{
	ModeCount:  "COUNT(*)",
	ModeFeed:   feedProjection,
	ModeSaved:  feedProjection,
	ModeStream: feedProjection,
}

// feedQueryBuilder accumulates the positional args in emission order while
// the SQL string is assembled piece by piece.
type feedQueryBuilder struct{ args []any }

// addArg records an arg and returns its placeholder ($N).
func (b *feedQueryBuilder) addArg(v any) string {
	b.args = append(b.args, v)
	return "?"
}

// BuildFeedQuery is the single dynamic SQL builder. It returns the SQL string
// and the positional argument slice. $1 is always userID. The caller MUST NOT
// reorder or omit the access-control join or processed_at predicate; they are
// emitted unconditionally for every mode.
func BuildFeedQuery(userID string, filters FeedFilters, mode FeedMode) (string, []any) {
	b := &feedQueryBuilder{args: []any{userID}}
	sql := "SELECT " + modeProjections[mode] + " FROM " + b.from(mode)
	sql += " WHERE " + strings.Join(b.where(filters, mode), " AND ")
	if tail := b.tail(mode, filters); tail != "" {
		sql += " " + tail
	}
	return sql, b.args
}

// from joins the base tables. Access-control join is mandatory and cannot be
// disabled. We use tracking_status IN ('active','paused') so paused channels
// remain readable (consistent with the source spec's "still see your data").
// ModeSaved inner-joins through user_comment_stars to scope the result set
// to only rows the caller has starred.
func (b *feedQueryBuilder) from(mode FeedMode) string {
	from := `comments c
		JOIN platform_channels pc ON pc.id = c.platform_channel_id`
	if mode != ModeSaved {
		return from
	}
	return from + `
		JOIN user_comment_stars ucs2 ON ucs2.comment_id = c.id
		   AND ucs2.user_id = $1`
}

func (b *feedQueryBuilder) where(filters FeedFilters, mode FeedMode) []string {
	where := []string{accessControlClause, "c.processed_at IS NOT NULL"}
	where = b.addCategoryFilter(where, filters, mode == ModeSaved)
	where = b.addInList(where, "c.platform_channel_id IN (", filters.Channels, ")")
	where = b.addInsightFilter(where, filters.InsightType)
	where = b.addIntentFilter(where, filters.Intent)
	where = b.addDateBounds(where, filters)
	return b.addSearchFilter(where, filters.Search)
}

func (b *feedQueryBuilder) addIntentFilter(where []string, intent string) []string {
	switch intent {
	case "ideas":
		return append(where, "(c.category = 'content_idea' OR EXISTS (SELECT 1 FROM json_each(COALESCE(c.actionable_insights, '[]')) i WHERE json_extract(i.value, '$.type') = 'video_idea'))")
	case "actionable":
		return append(where, "json_array_length(COALESCE(c.actionable_insights, '[]')) > 0")
	default:
		return where
	}
}

// addCategoryFilter applies the effective category list after the two-layer
// toxic rule (Decision 7).
func (b *feedQueryBuilder) addCategoryFilter(where []string, filters FeedFilters, saved bool) []string {
	if saved && !filters.CategoriesSpecified {
		return where
	}
	effectiveCats, explicitCats := applyToxicLayer(filters.Categories, filters.IncludeToxic)
	if filters.CategoriesSpecified {
		explicitCats = true
	}
	if !explicitCats {
		// Default: restrict to the high-signal category set.
		return b.addInList(where, "c.category IN (", HighSignalCategories, ")")
	}
	if len(effectiveCats) == 0 {
		// Caller passed only toxic/spam without include_toxic=true → strip
		// reduces to empty list. Return a contradiction so we yield zero
		// rows without dropping the explicit-filter intent.
		return append(where, "FALSE")
	}
	return b.addInList(where, "c.category IN (", effectiveCats, ")")
}

// addInList emits `prefix $?, $?, … suffix` with one positional arg per value.
func (b *feedQueryBuilder) addInList(where []string, prefix string, vals []string, suffix string) []string {
	if len(vals) == 0 {
		return where
	}
	ph := make([]string, 0, len(vals))
	for _, v := range vals {
		ph = append(ph, b.addArg(v))
	}
	return append(where, prefix+strings.Join(ph, ",")+suffix)
}

func (b *feedQueryBuilder) addInsightFilter(where []string, insightType string) []string {
	if insightType == "" {
		return where
	}
	return append(where, "EXISTS (SELECT 1 FROM json_each(COALESCE(c.actionable_insights, '[]')) i WHERE json_extract(i.value, '$.type') = "+b.addArg(insightType)+")")
}

func (b *feedQueryBuilder) addDateBounds(where []string, filters FeedFilters) []string {
	if filters.DateFrom != nil {
		where = append(where, "c.posted_at >= "+b.addArg(*filters.DateFrom))
	}
	if filters.DateTo != nil {
		where = append(where, "c.posted_at < "+b.addArg(*filters.DateTo))
	}
	return where
}

// addSearchFilter reuses a single positional arg for both LIKE patterns.
func (b *feedQueryBuilder) addSearchFilter(where []string, search *string) []string {
	if search == nil || *search == "" {
		return where
	}
	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(*search)
	ph := b.addArg(escaped)
	return append(where, "(c.body LIKE '%' || "+ph+" || '%' OR COALESCE(c.topic,'') LIKE '%' || "+ph+" || '%') ESCAPE '\\'")
}

// tail selects the ORDER BY / LIMIT / OFFSET shape for the mode.
func (b *feedQueryBuilder) tail(mode FeedMode, filters FeedFilters) string {
	if mode == ModeCount {
		return ""
	}
	if mode == ModeSaved {
		// sort overridden per spec
		return "ORDER BY ucs2.starred_at DESC" + b.addPaging(filters)
	}
	tail := sortClause(filters.Sort)
	if mode == ModeFeed {
		tail += b.addPaging(filters)
	}
	return tail // ModeStream: export streaming — no LIMIT, deterministic order.
}

// sortClause resolves the requested sort, falling back to newest.
func sortClause(sort string) string {
	if s, ok := AllowedSorts[sort]; ok {
		return s
	}
	return AllowedSorts["newest"]
}

func (b *feedQueryBuilder) addPaging(filters FeedFilters) string {
	return " LIMIT " + b.addArg(safeLimit(filters.Limit)) + " OFFSET " + b.addArg(safeOffset(filters.Offset))
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
