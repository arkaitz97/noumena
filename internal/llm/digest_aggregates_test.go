package llm

import (
	"testing"
	"time"

	"noumena/internal/db"
)

func ptrFloat(f float64) *float64 { return &f }
func ptrStr(s string) *string     { return &s }

func TestComputeSentimentAverage(t *testing.T) {
	if got := computeSentimentAverage(nil); got != 0 {
		t.Errorf("nil → 0 want, got %v", got)
	}
	cs := []*db.Comment{
		{SentimentScore: ptrFloat(1.0)},
		{SentimentScore: ptrFloat(-1.0)},
		{SentimentScore: ptrFloat(0.5)},
	}
	got := computeSentimentAverage(cs)
	want := (1.0 - 1.0 + 0.5) / 3.0
	if got != want {
		t.Errorf("avg got %v want %v", got, want)
	}
	// All-NULL comments → 0, not panic.
	cs = []*db.Comment{{}, {}}
	if got := computeSentimentAverage(cs); got != 0 {
		t.Errorf("all-NULL → 0 want, got %v", got)
	}
}

func TestComputeSentimentTrend(t *testing.T) {
	// No baseline.
	if d, p := computeSentimentTrend(0.5, 0); d != "stable" || p != 0 {
		t.Errorf("zero baseline want stable/0 got %s/%v", d, p)
	}
	// Improving (well above +1%).
	if d, _ := computeSentimentTrend(0.5, 0.3); d != "improving" {
		t.Errorf("0.3→0.5 want improving got %s", d)
	}
	// Declining.
	if d, _ := computeSentimentTrend(0.3, 0.5); d != "declining" {
		t.Errorf("0.5→0.3 want declining got %s", d)
	}
	// Stable: tiny change inside ±1.0 pp.
	if d, _ := computeSentimentTrend(0.5005, 0.5); d != "stable" {
		t.Errorf("0.5→0.5005 want stable got %s", d)
	}
}

func TestComputeSentimentQuality(t *testing.T) {
	if got := computeSentimentQuality(-1.0); got != 0 {
		t.Errorf("-1 → 0 got %d", got)
	}
	if got := computeSentimentQuality(1.0); got != 100 {
		t.Errorf("1 → 100 got %d", got)
	}
	if got := computeSentimentQuality(0.0); got != 50 {
		t.Errorf("0 → 50 got %d", got)
	}
}

func TestComputeHealthScore(t *testing.T) {
	got := computeHealthScore(HealthScoreBreakdown{ResponseRate: 60, SentimentQuality: 90, EngagementDepth: 30})
	if got != 60 {
		t.Errorf("equal-weight mean want 60 got %d", got)
	}
}

func TestComputeEngagementDepth_EdgeCases(t *testing.T) {
	if got := computeEngagementDepth(nil); got != 0 {
		t.Errorf("nil → 0 got %d", got)
	}
	// Single short comment, single topic.
	one := &db.Comment{Body: "hi", Topic: ptrStr("t1")}
	got := computeEngagementDepth([]*db.Comment{one})
	if got < 0 || got > 100 {
		t.Errorf("range violated got %d", got)
	}
}

func TestComputeTopTopics(t *testing.T) {
	cs := []*db.Comment{
		{Topic: ptrStr("a")}, {Topic: ptrStr("a")}, {Topic: ptrStr("b")},
		{Topic: ptrStr("c")}, {Topic: nil},
	}
	got := computeTopTopics(cs, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 topics got %d", len(got))
	}
	if got[0].Topic != "a" || got[0].Count != 2 {
		t.Errorf("rank-1 want a/2 got %+v", got[0])
	}
	// Tie between b and c → alphabetical: b first.
	if got[1].Topic != "b" {
		t.Errorf("tiebreak alpha want b got %s", got[1].Topic)
	}
}

func TestComputeCategoryDistribution(t *testing.T) {
	cs := []*db.Comment{
		{Category: ptrStr("praise")}, {Category: ptrStr("praise")},
		{Category: ptrStr("question")}, {Category: nil},
	}
	got := computeCategoryDistribution(cs)
	if got["praise"] != 2 || got["question"] != 1 {
		t.Errorf("distribution got %+v", got)
	}
	if _, ok := got[""]; ok {
		t.Errorf("nil categories should be skipped")
	}
}

func TestSelectSampleComments(t *testing.T) {
	now := time.Now()
	cs := []*db.Comment{
		{Body: "neutral", SentimentScore: ptrFloat(0.0), PostedAt: now},
		{Body: "very-negative", SentimentScore: ptrFloat(-0.95), PostedAt: now},
		{Body: "very-positive", SentimentScore: ptrFloat(0.9), PostedAt: now.Add(-time.Hour)},
		{Body: "no-score", PostedAt: now},
	}
	got := selectSampleComments(cs, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 got %d", len(got))
	}
	// Expect very-negative first (|0.95|), very-positive second (|0.9|).
	if got[0] != "very-negative" || got[1] != "very-positive" {
		t.Errorf("ranking got %+v", got)
	}
}
