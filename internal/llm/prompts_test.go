package llm

import (
	"reflect"
	"testing"
)

func TestParseClassificationResponse_WellFormed(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"great video","actionable_insights":[]}]`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []LLMResult{{CommentIndex: 0, Category: "praise", Confidence: 0.9, Topic: "great video", ActionableInsights: []Insight{}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestParseClassificationResponse_WithInsights(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"great video","actionable_insights":[{"type":"video_idea","description":"Make a tutorial on color grading","confidence":0.85}]}]`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got[0].ActionableInsights) != 1 {
		t.Fatalf("expected 1 insight, got %d", len(got[0].ActionableInsights))
	}
	if got[0].ActionableInsights[0].Type != "video_idea" {
		t.Errorf("want video_idea, got %s", got[0].ActionableInsights[0].Type)
	}
}

// 8.8 — markdown-wrapped JSON tolerated.
func TestParseClassificationResponse_MarkdownFenced(t *testing.T) {
	raw := "```json\n[{\"comment_index\":0,\"category\":\"question\",\"confidence\":0.8,\"topic\":\"x\",\"actionable_insights\":[]}]\n```"
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Category != "question" {
		t.Errorf("category = %s", got[0].Category)
	}
}

func TestParseClassificationResponse_TrailingCommentary(t *testing.T) {
	raw := `[{"comment_index":0,"category":"spam","confidence":0.99,"topic":"buy crypto","actionable_insights":[]}]

Hope this helps!`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Category != "spam" {
		t.Errorf("category = %s", got[0].Category)
	}
}

func TestParseClassificationResponse_InvalidCategory(t *testing.T) {
	raw := `[{"comment_index":0,"category":"compliment","confidence":0.9,"topic":"x","actionable_insights":[]}]`
	if _, err := ParseClassificationResponse(raw, 1); err == nil {
		t.Fatal("expected error for invalid category")
	}
}

func TestParseClassificationResponse_InvalidInsightTypeSkipped(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"x","actionable_insights":[{"type":"invalid_type","description":"bad","confidence":0.8}]}]`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got[0].ActionableInsights) != 0 {
		t.Errorf("expected 0 insights, got %d", len(got[0].ActionableInsights))
	}
}

func TestParseClassificationResponse_Max5Insights(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"x","actionable_insights":[
		{"type":"video_idea","description":"idea 1","confidence":0.9},
		{"type":"video_idea","description":"idea 2","confidence":0.9},
		{"type":"video_idea","description":"idea 3","confidence":0.9},
		{"type":"video_idea","description":"idea 4","confidence":0.9},
		{"type":"video_idea","description":"idea 5","confidence":0.9},
		{"type":"video_idea","description":"idea 6","confidence":0.9},
		{"type":"video_idea","description":"idea 7","confidence":0.9}
	]}]`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got[0].ActionableInsights) != 5 {
		t.Errorf("expected 5 insights, got %d", len(got[0].ActionableInsights))
	}
}

func TestParseClassificationResponse_DescriptionTruncated(t *testing.T) {
	longDesc := ""
	for i := 0; i < 250; i++ {
		longDesc += "a"
	}
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"x","actionable_insights":[{"type":"video_idea","description":"` + longDesc + `","confidence":0.8}]}]`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got[0].ActionableInsights[0].Description) > maxInsightDescriptionLen {
		t.Errorf("description not truncated: len=%d", len(got[0].ActionableInsights[0].Description))
	}
}

func TestParseClassificationResponse_TopicTruncated(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"one two three four five six seven eight nine ten eleven twelve","actionable_insights":[]}]`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Topic != "one two three four five six seven eight nine ten" {
		t.Errorf("topic not truncated to 10 words: %q", got[0].Topic)
	}
}

func TestParseClassificationResponse_CountMismatch(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"x","actionable_insights":[]}]`
	if _, err := ParseClassificationResponse(raw, 2); err == nil {
		t.Fatal("expected count-mismatch error")
	}
}

func TestParseClassificationResponse_NoArray(t *testing.T) {
	raw := `the model decided to chat instead of returning JSON`
	if _, err := ParseClassificationResponse(raw, 1); err == nil {
		t.Fatal("expected error for missing array")
	}
}

func TestApplyConfidenceThreshold_Downgrade(t *testing.T) {
	in := []LLMResult{
		{CommentIndex: 0, Category: "praise", Confidence: 0.55, Topic: "x", ActionableInsights: []Insight{{Type: "video_idea", Description: "test", Confidence: 0.8}}},
		{CommentIndex: 1, Category: "question", Confidence: 0.95, Topic: "y", ActionableInsights: []Insight{{Type: "video_idea", Description: "test", Confidence: 0.5}}},
	}
	out := ApplyConfidenceThreshold(in, 0.7)
	if out[0].Category != "other" || out[0].Confidence != 0.55 {
		t.Errorf("want low-confidence downgraded with confidence preserved, got %+v", out[0])
	}
	if out[1].Category != "question" {
		t.Errorf("want above-threshold kept, got %+v", out[1])
	}
	// Insight below threshold should be filtered
	if len(out[1].ActionableInsights) != 0 {
		t.Errorf("want insight filtered, got %+v", out[1].ActionableInsights)
	}
	// Insight above threshold should be kept
	if len(out[0].ActionableInsights) != 1 {
		t.Errorf("want insight kept, got %+v", out[0].ActionableInsights)
	}
}

func TestApplyConfidenceThreshold_AtThresholdKept(t *testing.T) {
	in := []LLMResult{{CommentIndex: 0, Category: "question", Confidence: 0.7, Topic: "x", ActionableInsights: []Insight{{Type: "video_idea", Description: "test", Confidence: 0.7}}}}
	out := ApplyConfidenceThreshold(in, 0.7)
	if out[0].Category != "question" {
		t.Errorf("want at-threshold kept, got %+v", out[0])
	}
	if len(out[0].ActionableInsights) != 1 {
		t.Errorf("want insight at threshold kept, got %+v", out[0].ActionableInsights)
	}
}

func TestBuildClassificationPrompt_TruncatesAndEnumerates(t *testing.T) {
	long := make([]byte, maxCommentBytes+500)
	for i := range long {
		long[i] = 'a'
	}
	cs := []PromptComment{{Body: "hello"}, {Body: string(long)}}
	sys, user := BuildClassificationPrompt(cs)
	if sys == "" {
		t.Fatal("system prompt empty")
	}
	if len(user) > 2*maxCommentBytes+200 {
		t.Errorf("user prompt longer than expected: %d", len(user))
	}
	// Check zero-based index marker.
	if !contains(user, "[0]") || !contains(user, "[1]") {
		t.Errorf("indices missing in user prompt: %q", user)
	}
}

func TestBuildClassificationPrompt_CustomPromptPrepended(t *testing.T) {
	cs := []PromptComment{
		{Body: "hello", CustomPrompt: "Focus on sentiment only"},
		{Body: "world"},
	}
	_, user := BuildClassificationPrompt(cs)
	if !contains(user, "Additional context for classification: Focus on sentiment only") {
		t.Errorf("custom prompt not prepended: %q", user)
	}
}

func TestBuildClassificationPrompt_EmptyCustomPromptOmitted(t *testing.T) {
	cs := []PromptComment{
		{Body: "hello", CustomPrompt: ""},
	}
	_, user := BuildClassificationPrompt(cs)
	if contains(user, "Additional context for classification:") {
		t.Errorf("empty custom prompt should not appear: %q", user)
	}
}

func TestBuildClassificationPrompt_ParentContextIncluded(t *testing.T) {
	parent := "This is the parent comment text"
	cs := []PromptComment{
		{Body: "I agree with that", ParentContext: parent},
	}
	_, user := BuildClassificationPrompt(cs)
	if !contains(user, "reply to: "+parent) {
		t.Errorf("parent context missing: %q", user)
	}
}

func TestBuildClassificationPrompt_ParentContextTruncatedTo200(t *testing.T) {
	longParent := ""
	for i := 0; i < 300; i++ {
		longParent += "x"
	}
	cs := []PromptComment{
		{Body: "reply", ParentContext: longParent},
	}
	_, user := BuildClassificationPrompt(cs)
	// parent should be truncated to 200 + "..." = 203 chars
	expectedParent := longParent[:200] + "..."
	expected := "(reply to: " + expectedParent + ") reply"
	if !contains(user, expected) {
		t.Errorf("parent context not truncated correctly.\nExpected to contain: %q\nGot: %q", expected, user)
	}
}

func TestBuildClassificationPrompt_NoParentContext(t *testing.T) {
	cs := []PromptComment{
		{Body: "standalone comment"},
	}
	_, user := BuildClassificationPrompt(cs)
	if contains(user, "reply to:") {
		t.Errorf("should not have parent context marker: %q", user)
	}
	if !contains(user, "[0] standalone comment") {
		t.Errorf("comment body missing: %q", user)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && (indexOf(s, sub) >= 0)))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
