package llm

import (
	"reflect"
	"testing"
)

func TestParseClassificationResponse_WellFormed(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"great video","sentiment":0.85}]`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []LLMResult{{CommentIndex: 0, Category: "praise", Confidence: 0.9, Topic: "great video", Sentiment: 0.85}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
}

// 8.8 — markdown-wrapped JSON tolerated.
func TestParseClassificationResponse_MarkdownFenced(t *testing.T) {
	raw := "```json\n[{\"comment_index\":0,\"category\":\"question\",\"confidence\":0.8,\"topic\":\"x\",\"sentiment\":0.0}]\n```"
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Category != "question" {
		t.Errorf("category = %s", got[0].Category)
	}
}

func TestParseClassificationResponse_TrailingCommentary(t *testing.T) {
	raw := `[{"comment_index":0,"category":"spam","confidence":0.99,"topic":"buy crypto","sentiment":-0.3}]

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
	raw := `[{"comment_index":0,"category":"compliment","confidence":0.9,"topic":"x","sentiment":0.5}]`
	if _, err := ParseClassificationResponse(raw, 1); err == nil {
		t.Fatal("expected error for invalid category")
	}
}

// 8.9 — out-of-range sentiment => parse failure.
func TestParseClassificationResponse_OutOfRangeSentiment(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"x","sentiment":1.7}]`
	if _, err := ParseClassificationResponse(raw, 1); err == nil {
		t.Fatal("expected error for sentiment out of range")
	}
}

func TestParseClassificationResponse_TopicTruncated(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"one two three four five six seven eight nine ten eleven twelve","sentiment":0.5}]`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Topic != "one two three four five six seven eight nine ten" {
		t.Errorf("topic not truncated to 10 words: %q", got[0].Topic)
	}
}

func TestParseClassificationResponse_SentimentRoundedTo2Decimals(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"x","sentiment":0.8473}]`
	got, err := ParseClassificationResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Sentiment != 0.85 {
		t.Errorf("want 0.85, got %v", got[0].Sentiment)
	}
}

func TestParseClassificationResponse_CountMismatch(t *testing.T) {
	raw := `[{"comment_index":0,"category":"praise","confidence":0.9,"topic":"x","sentiment":0.5}]`
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
		{CommentIndex: 0, Category: "praise", Confidence: 0.55, Topic: "x", Sentiment: 0},
		{CommentIndex: 1, Category: "question", Confidence: 0.95, Topic: "y", Sentiment: 0},
	}
	out := ApplyConfidenceThreshold(in, 0.7)
	if out[0].Category != "other" || out[0].Confidence != 0.55 {
		t.Errorf("want low-confidence downgraded with confidence preserved, got %+v", out[0])
	}
	if out[1].Category != "question" {
		t.Errorf("want above-threshold kept, got %+v", out[1])
	}
}

func TestApplyConfidenceThreshold_AtThresholdKept(t *testing.T) {
	in := []LLMResult{{CommentIndex: 0, Category: "question", Confidence: 0.7, Topic: "x", Sentiment: 0}}
	out := ApplyConfidenceThreshold(in, 0.7)
	if out[0].Category != "question" {
		t.Errorf("want at-threshold kept, got %+v", out[0])
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
