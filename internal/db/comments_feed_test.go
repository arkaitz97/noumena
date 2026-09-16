package db

import (
	"context"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestBuildFeedQuery_DefaultCategories(t *testing.T) {
	sql, args := BuildFeedQuery("u1", FeedFilters{}, ModeFeed)

	for _, cat := range HighSignalCategories {
		found := false
		for _, a := range args {
			if s, ok := a.(string); ok && s == cat {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("default category %q not bound as parameter", cat)
		}
	}
	for cat := range ToxicCategories {
		for _, a := range args {
			if s, ok := a.(string); ok && s == cat {
				t.Errorf("toxic category %q leaked into default scope", cat)
			}
		}
	}
	if !strings.Contains(sql, "c.processed_at IS NOT NULL") {
		t.Errorf("processed_at predicate missing")
	}
	if !strings.Contains(sql, "tracking_status IN ('active','paused')") {
		t.Errorf("access-control join missing")
	}
}

func TestBuildFeedQuery_IncludeToxicTwoLayer(t *testing.T) {
	cases := []struct {
		name         string
		categories   []string
		includeToxic bool
		wantToxic    bool
	}{
		{"flag alone is inert", nil, true, false},
		{"explicit toxic without flag is stripped", []string{"toxic_criticism", "spam"}, false, false},
		{"explicit toxic with flag passes", []string{"toxic_criticism", "spam"}, true, true},
		{"mixed without flag strips toxic", []string{"question", "toxic_criticism"}, false, false},
		{"mixed with flag passes toxic", []string{"question", "toxic_criticism"}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, args := BuildFeedQuery("u1", FeedFilters{
				Categories:   tc.categories,
				IncludeToxic: tc.includeToxic,
			}, ModeFeed)
			leaked := false
			for _, a := range args {
				if s, ok := a.(string); ok {
					if _, isToxic := ToxicCategories[s]; isToxic {
						leaked = true
					}
				}
			}
			if leaked != tc.wantToxic {
				t.Errorf("toxic-leak: got=%v want=%v args=%v", leaked, tc.wantToxic, args)
			}
		})
	}
}

func TestBuildFeedQuery_SortModes(t *testing.T) {
	cases := map[string]string{
		"newest": "ORDER BY c.posted_at DESC",
		"oldest": "ORDER BY c.posted_at ASC",
		"":       "ORDER BY c.posted_at DESC", // default
		"bogus":  "ORDER BY c.posted_at DESC", // fallback
	}
	for sort, want := range cases {
		sql, _ := BuildFeedQuery("u1", FeedFilters{Sort: sort}, ModeFeed)
		if !strings.Contains(sql, want) {
			t.Errorf("sort=%q: missing %q in %s", sort, want, sql)
		}
	}
}

func TestBuildFeedQuery_ModeSaved_OverridesSort(t *testing.T) {
	sql, _ := BuildFeedQuery("u1", FeedFilters{Sort: "newest"}, ModeSaved)
	if !strings.Contains(sql, "ORDER BY ucs2.starred_at DESC") {
		t.Errorf("saved mode did not force starred_at DESC: %s", sql)
	}
	if !strings.Contains(sql, "JOIN user_comment_stars ucs2") {
		t.Errorf("saved mode missing inner join through user_comment_stars: %s", sql)
	}
}

func TestBuildFeedQuery_ModeCount_NoOrderLimit(t *testing.T) {
	sql, _ := BuildFeedQuery("u1", FeedFilters{Limit: 50}, ModeCount)
	if !strings.HasPrefix(sql, "SELECT COUNT(*)") {
		t.Errorf("count mode projection: %s", sql)
	}
	if strings.Contains(sql, "ORDER BY") || strings.Contains(sql, "LIMIT") {
		t.Errorf("count mode must omit ORDER BY/LIMIT: %s", sql)
	}
}

func TestBuildFeedQuery_ModeStream_NoLimit(t *testing.T) {
	sql, _ := BuildFeedQuery("u1", FeedFilters{}, ModeStream)
	if strings.Contains(sql, "LIMIT") || strings.Contains(sql, "OFFSET") {
		t.Errorf("stream mode must omit LIMIT/OFFSET: %s", sql)
	}
}

// TestBuildFeedQuery_NoSQLi enforces task 3.13: every user-controlled value
// is passed as a positional parameter; the SQL string itself contains no
// caller-supplied substring.
func TestBuildFeedQuery_NoSQLi(t *testing.T) {
	evil := "x'; DROP TABLE users; --"
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	sql, args := BuildFeedQuery(evil, FeedFilters{
		Channels:     []string{evil},
		Categories:   []string{evil},
		IncludeToxic: true,
		InsightType:  "video_idea",
		DateFrom:     &from,
		DateTo:       &to,
		Search:       &evil,
		Limit:        50,
		Offset:       100,
		Sort:         "newest",
	}, ModeFeed)

	if strings.Contains(sql, "DROP TABLE") || strings.Contains(sql, evil) {
		t.Fatalf("SQL contains user-controlled substring (SQLi risk): %s", sql)
	}

	// The evil string should appear three times in args (userID, channel,
	// category) plus once for search.
	count := 0
	for _, a := range args {
		if s, ok := a.(string); ok && s == evil {
			count++
		}
	}
	if count < 3 {
		t.Errorf("expected user input bound at least 3x as parameters, got %d (args=%v)", count, args)
	}
}

func TestBuildFeedQuery_FilterCombinations(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	q := "lighting"
	sql, args := BuildFeedQuery("u1", FeedFilters{
		Channels:    []string{"ch1", "ch2"},
		Categories:  []string{"question"},
		InsightType: "video_idea",
		DateFrom:    &from,
		Search:      &q,
		Limit:       50,
		Offset:      0,
		Sort:        "newest",
	}, ModeFeed)
	if !strings.Contains(sql, "c.platform_channel_id IN") {
		t.Errorf("channels filter missing")
	}
	if !strings.Contains(sql, "EXISTS (SELECT 1 FROM json_each") {
		t.Errorf("insight_type filter missing")
	}
	if !strings.Contains(sql, "c.posted_at >=") {
		t.Errorf("date_from filter missing")
	}
	if !strings.Contains(sql, "c.body LIKE") {
		t.Errorf("search filter missing")
	}
	if len(args) < 6 {
		t.Errorf("expected several args, got %d", len(args))
	}
}

func TestBuildFeedQuery_IntentTruthTable(t *testing.T) {
	for _, tc := range []struct {
		intent, want string
	}{
		{"all", "processed_at IS NOT NULL"},
		{"ideas", "c.category = 'content_idea' OR EXISTS"},
		{"actionable", "json_array_length"},
	} {
		sql, _ := BuildFeedQuery("u1", FeedFilters{Intent: tc.intent}, ModeFeed)
		if !strings.Contains(sql, tc.want) {
			t.Errorf("intent %q missing %q", tc.intent, tc.want)
		}
	}
}

func TestBuildFeedQuery_DateAndSearchBoundaries(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	search := `%_\\`
	sql, args := BuildFeedQuery("u1", FeedFilters{DateFrom: &from, DateTo: &to, Search: &search}, ModeFeed)
	if !strings.Contains(sql, "c.posted_at >=") || !strings.Contains(sql, "c.posted_at <") || !strings.Contains(sql, `ESCAPE '\'`) {
		t.Fatalf("boundary predicates missing: %s", sql)
	}
	if !containsArg(args, `\%\_\\\\`) {
		t.Fatalf("search was not escaped: %#v", args)
	}
}

func containsArg(args []any, want string) bool {
	for _, arg := range args {
		if s, ok := arg.(string); ok && s == want {
			return true
		}
	}
	return false
}

func TestBuildFeedQuery_SavedDefaultsHaveNoHiddenCategoryFilter(t *testing.T) {
	sql, args := BuildFeedQuery("u1", FeedFilters{}, ModeSaved)
	if strings.Contains(sql, "c.category IN") {
		t.Fatalf("saved default has category filter: %s", sql)
	}
	for _, cat := range HighSignalCategories {
		for _, arg := range args {
			if arg == cat {
				t.Fatalf("saved default bound hidden category %q", cat)
			}
		}
	}
}

func TestInsightAggregatesRespectCompleteScope(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	sql, _ := BuildFeedQuery("u1", FeedFilters{Channels: []string{"ch1", "ch2"}, Categories: []string{"question"}, CategoriesSpecified: true, DateFrom: &from, DateTo: &to}, ModeCount)
	for _, want := range []string{"c.platform_channel_id IN", "c.category IN", "c.posted_at >=", "c.posted_at <"} {
		if !strings.Contains(sql, want) {
			t.Errorf("aggregate scope missing %q", want)
		}
	}
}

func seedGrantChannelFeed(t *testing.T, ctx context.Context, sq *SQLiteQuery) {
	t.Helper()
	db := sq.DB()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, sql, args...); err != nil {
			t.Fatalf("seed %s: %v", sql, err)
		}
	}
	mustExec(`INSERT INTO users (id, email) VALUES ('u-owner', 'owner@example.com'), ('u-member', 'member@example.com'), ('u-stranger', 'stranger@example.com')`)
	mustExec(`INSERT INTO workspaces (id, name, created_by) VALUES ('ws1', 'ws', 'u-owner')`)
	mustExec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('ws1', 'u-owner', 'admin'), ('ws1', 'u-member', 'viewer')`)
	mustExec(`INSERT INTO platform_channels (id, platform, platform_channel_id, display_name) VALUES ('ch1', 'youtube', 'yt-1', 'Chan')`)
	mustExec(`INSERT INTO user_channel_tracking (id, user_id, platform_channel_id, access_token_enc, refresh_token_enc) VALUES ('tr-owner', 'u-owner', 'ch1', 'enc', 'enc')`)
	mustExec(`INSERT INTO comments (id, platform_channel_id, platform_comment_id, body, posted_at, processed_at, category) VALUES ('cm1', 'ch1', 'pc1', 'body', '2026-09-01T00:00:00Z', '2026-09-02T00:00:00Z', 'question')`)
}

func TestGrantChannel_MemberSeesSharedFeed(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)
	sq := q.(*SQLiteQuery)
	seedGrantChannelFeed(t, ctx, sq)
	if err := sq.GrantChannel(ctx, "ws1", "ch1", "u-owner"); err != nil {
		t.Fatalf("GrantChannel: %v", err)
	}
	rows, err := sq.QueryCommentFeed(ctx, "u-member", FeedFilters{Limit: 50}, 5*time.Second)
	if err != nil {
		t.Fatalf("QueryCommentFeed: %v", err)
	}
	for _, r := range rows {
		if r.ID == "cm1" {
			return
		}
	}
	t.Fatalf("member feed missing cm1 (got %d rows)", len(rows))
}

func TestGrantChannel_NonMemberDeniedFeed(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)
	sq := q.(*SQLiteQuery)
	seedGrantChannelFeed(t, ctx, sq)
	rows, err := sq.QueryCommentFeed(ctx, "u-stranger", FeedFilters{Limit: 50}, 5*time.Second)
	if err != nil {
		t.Fatalf("QueryCommentFeed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("stranger feed not empty (got %d rows)", len(rows))
	}
	ok, err := sq.UserHasAccessToComment(ctx, "u-stranger", "cm1")
	if err != nil {
		t.Fatalf("UserHasAccessToComment: %v", err)
	}
	if ok {
		t.Fatalf("stranger has access to cm1")
	}
}

func TestGrantChannel_StarWithoutAccessDenied(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)
	sq := q.(*SQLiteQuery)
	seedGrantChannelFeed(t, ctx, sq)
	if err := sq.GrantChannel(ctx, "ws1", "ch1", "u-owner"); err != nil {
		t.Fatalf("GrantChannel: %v", err)
	}
	ok, err := sq.StarCommentScoped(ctx, "u-stranger", "cm1")
	if err != nil {
		t.Fatalf("StarCommentScoped stranger: %v", err)
	}
	if ok {
		t.Fatalf("stranger star allowed without access")
	}
	ok, err = sq.StarCommentScoped(ctx, "u-member", "cm1")
	if err != nil {
		t.Fatalf("StarCommentScoped member: %v", err)
	}
	if !ok {
		t.Fatalf("member star denied after grant")
	}
}

func TestBulkStar_ForeignDenied(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	q := NewSQLiteQuery(database)
	sq := q.(*SQLiteQuery)
	seedGrantChannelFeed(t, ctx, sq)
	if err := sq.GrantChannel(ctx, "ws1", "ch1", "u-owner"); err != nil {
		t.Fatalf("GrantChannel: %v", err)
	}
	n, err := sq.BulkStarComments(ctx, "u-stranger", []string{"cm1"})
	if err != nil {
		t.Fatalf("BulkStarComments stranger: %v", err)
	}
	if n != 0 {
		t.Fatalf("stranger bulk star allowed without access (got %d)", n)
	}
	var strangerStars int
	if err := sq.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM user_comment_stars WHERE user_id = 'u-stranger' AND comment_id = 'cm1'`).Scan(&strangerStars); err != nil {
		t.Fatalf("count stranger stars: %v", err)
	}
	if strangerStars != 0 {
		t.Fatalf("stranger star row present without access")
	}
	n, err = sq.BulkStarComments(ctx, "u-member", []string{"cm1"})
	if err != nil {
		t.Fatalf("BulkStarComments member: %v", err)
	}
	if n != 1 {
		t.Fatalf("member bulk star denied after grant (got %d)", n)
	}
	n, err = sq.BulkUnstarComments(ctx, "u-stranger", []string{"cm1"})
	if err != nil {
		t.Fatalf("BulkUnstarComments stranger: %v", err)
	}
	if n != 0 {
		t.Fatalf("stranger bulk unstar affected rows without access (got %d)", n)
	}
	var memberStars int
	if err := sq.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM user_comment_stars WHERE user_id = 'u-member' AND comment_id = 'cm1'`).Scan(&memberStars); err != nil {
		t.Fatalf("count member stars: %v", err)
	}
	if memberStars != 1 {
		t.Fatalf("member star removed by stranger unstar (got %d)", memberStars)
	}
}
