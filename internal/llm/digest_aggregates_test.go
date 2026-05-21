package llm

import (
	"encoding/json"
	"testing"

	"noumena/internal/db"
)

func ptrFloat(f float64) *float64 { return &f }
func ptrStr(s string) *string     { return &s }

func TestComputeTopActionableInsights(t *testing.T) {
	insights := []Insight{
		{Type: "video_idea", Description: "Make a tutorial", Confidence: 0.9},
		{Type: "video_idea", Description: "Make a tutorial", Confidence: 0.8},
		{Type: "editing_tip", Description: "Use crossfade", Confidence: 0.7},
	}
	b, _ := json.Marshal(insights)
	cs := []*db.Comment{
		{ActionableInsights: b},
		{ActionableInsights: b},
		{ActionableInsights: []byte("[]")},
	}
	got := computeTopActionableInsights(cs, 5)
	if len(got) != 2 {
		t.Fatalf("want 2 insights got %d", len(got))
	}
	if got[0].Type != "video_idea" || got[0].Count != 4 {
		t.Errorf("rank-1 want video_idea/4 got %+v", got[0])
	}
}

func TestComputeTopActionableInsights_Empty(t *testing.T) {
	if got := computeTopActionableInsights(nil, 5); got != nil {
		t.Errorf("nil → nil want, got %v", got)
	}
}

func TestComputeHealthScore(t *testing.T) {
	got := computeHealthScore(HealthScoreBreakdown{ResponseRate: 60, EngagementDepth: 30})
	if got != 45 {
		t.Errorf("equal-weight mean want 45 got %d", got)
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
	cs := []*db.Comment{
		{Body: "high-conf", Confidence: ptrFloat(0.95)},
		{Body: "low-conf", Confidence: ptrFloat(0.3)},
		{Body: "no-conf"},
	}
	got := selectSampleComments(cs, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 got %d", len(got))
	}
	// Expect high-conf first, low-conf second.
	if got[0] != "high-conf" || got[1] != "low-conf" {
		t.Errorf("ranking got %+v", got)
	}
}
