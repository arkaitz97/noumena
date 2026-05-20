package db

import (
	"strings"
	"testing"
	"time"
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
		"newest":         "ORDER BY c.posted_at DESC",
		"sentiment_desc": "ORDER BY c.sentiment_score DESC NULLS LAST",
		"sentiment_asc":  "ORDER BY c.sentiment_score ASC NULLS LAST",
		"":               "ORDER BY c.posted_at DESC", // default
		"bogus":          "ORDER BY c.posted_at DESC", // fallback
	}
	for sort, want := range cases {
		sql, _ := BuildFeedQuery("u1", FeedFilters{Sort: sort}, ModeFeed)
		if !strings.Contains(sql, want) {
			t.Errorf("sort=%q: missing %q in %s", sort, want, sql)
		}
	}
}

func TestBuildFeedQuery_ModeSaved_OverridesSort(t *testing.T) {
	sql, _ := BuildFeedQuery("u1", FeedFilters{Sort: "sentiment_desc"}, ModeSaved)
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
	min := -0.5
	max := 0.9

	sql, args := BuildFeedQuery(evil, FeedFilters{
		Channels:     []string{evil},
		Categories:   []string{evil},
		IncludeToxic: true,
		SentimentMin: &min,
		SentimentMax: &max,
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
	min := 0.5
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	q := "lighting"
	sql, args := BuildFeedQuery("u1", FeedFilters{
		Channels:     []string{"ch1", "ch2"},
		Categories:   []string{"question"},
		SentimentMin: &min,
		DateFrom:     &from,
		Search:       &q,
		Limit:        50,
		Offset:       0,
		Sort:         "newest",
	}, ModeFeed)
	if !strings.Contains(sql, "c.platform_channel_id IN") {
		t.Errorf("channels filter missing")
	}
	if !strings.Contains(sql, "c.sentiment_score >=") {
		t.Errorf("sentiment_min filter missing")
	}
	if !strings.Contains(sql, "c.posted_at >=") {
		t.Errorf("date_from filter missing")
	}
	if !strings.Contains(sql, "c.body ILIKE") {
		t.Errorf("search filter missing")
	}
	if len(args) < 6 {
		t.Errorf("expected several args, got %d", len(args))
	}
}
