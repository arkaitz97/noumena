package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"noumena/internal/config"
)

func TestParseFeedFiltersContract(t *testing.T) {
	cfg := &config.Config{FeedDefaultLimit: 20, FeedMaxLimit: 100}
	for _, tc := range []struct {
		name, raw  string
		categories bool
		intent     string
	}{
		{"omitted categories", "", false, ""},
		{"explicit empty", "categories=", true, ""},
		{"ideas", "intent=ideas&categories=question&date_to=2026-09-02", true, "ideas"},
	} {
		r := httptest.NewRequest("GET", "/feed?"+tc.raw, nil)
		got, err := parseFeedFilters(r, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got.CategoriesSpecified != tc.categories || got.Intent != tc.intent {
			t.Errorf("%s: %+v", tc.name, got)
		}
		if tc.name == "omitted categories" && len(got.Categories) != 0 {
			t.Fatalf("omitted categories should defer defaults")
		}
		if tc.name == "explicit empty" && len(got.Categories) != 0 {
			t.Fatalf("explicit empty parsed as categories")
		}
		if tc.name == "ideas" && !got.DateTo.Equal(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("date_to not exclusive next day: %v", got.DateTo)
		}
	}
}

func TestParseFeedFiltersLiteralSearch(t *testing.T) {
	cfg := &config.Config{FeedDefaultLimit: 20, FeedMaxLimit: 100}
	r := httptest.NewRequest("GET", "/feed?search=%25_%5C", nil)
	got, err := parseFeedFilters(r, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Search == nil || *got.Search != "%_\\" {
		t.Fatalf("search decoded incorrectly: %q", *got.Search)
	}
}

func TestAggregateScopeUsesAllSelectedChannels(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	f := aggregateFilters(nil, 30)
	if len(f.Channels) != 0 || f.DateFrom == nil {
		t.Fatalf("all-channel aggregate lost scope: %+v", f)
	}
	if strings.TrimSpace(f.Intent) != "" || !f.CategoriesSpecified {
		t.Fatalf("aggregate intent/category scope changed: %+v", f)
	}
	_ = from
}
