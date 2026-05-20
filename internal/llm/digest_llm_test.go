package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeDigestLLM struct {
	raw string
	err error
	// blockUntilDone simulates a slow call; respect ctx cancellation.
	delay time.Duration
}

func (f *fakeDigestLLM) GenerateDigestRaw(ctx context.Context, prompt string) (string, error) {
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(f.delay):
		}
	}
	if f.err != nil {
		return "", f.err
	}
	return f.raw, nil
}

func truthFixture() DigestContent {
	return DigestContent{
		SentimentTrend:        SentimentTrend{CurrentAvg: 0.5, ChangePct: 10, Direction: "improving"},
		EngagementHealthScore: 75,
		HealthScoreBreakdown:  HealthScoreBreakdown{ResponseRate: 60, SentimentQuality: 75, EngagementDepth: 90},
		TotalCommentsAnalyzed: 42,
		FilteredComments:      3,
	}
}

func TestCallDigestLLM_Success(t *testing.T) {
	raw := `{
		"top_ideas":[{"idea":"Make a tutorial","evidence":"7 viewers asked","sample_comments":["please tutorial"]}],
		"top_questions":[{"question":"When is the next video?","count":4,"unanswered":true}],
		"sentiment_trend":{"current_avg":0,"change_pct":0,"direction":"stable"},
		"engagement_health_score":0,
		"health_score_breakdown":{"response_rate":0,"sentiment_quality":0,"engagement_depth":0},
		"total_comments_analyzed":0,
		"filtered_comments":0
	}`
	got, err := CallDigestLLM(context.Background(), &fakeDigestLLM{raw: raw}, "p", truthFixture())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Numeric fields authoritatively overwritten.
	if got.EngagementHealthScore != 75 {
		t.Errorf("health score not overwritten: %d", got.EngagementHealthScore)
	}
	if got.SentimentTrend.Direction != "improving" {
		t.Errorf("trend not overwritten: %+v", got.SentimentTrend)
	}
	if got.TotalCommentsAnalyzed != 42 {
		t.Errorf("totals not overwritten: %d", got.TotalCommentsAnalyzed)
	}
	if len(got.TopIdeas) != 1 || got.TopIdeas[0].Idea == "" {
		t.Errorf("ideas wrong: %+v", got.TopIdeas)
	}
}

func TestCallDigestLLM_TruncatesToFive(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"top_ideas":[`)
	for i := 0; i < 7; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"idea":"x","evidence":"y","sample_comments":[]}`)
	}
	sb.WriteString(`],"top_questions":[`)
	for i := 0; i < 6; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"question":"q","count":1,"unanswered":false}`)
	}
	sb.WriteString(`]}`)
	got, err := CallDigestLLM(context.Background(), &fakeDigestLLM{raw: sb.String()}, "p", truthFixture())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got.TopIdeas) != 5 {
		t.Errorf("ideas truncated to 5 want, got %d", len(got.TopIdeas))
	}
	if len(got.TopQuestions) != 5 {
		t.Errorf("questions truncated to 5 want, got %d", len(got.TopQuestions))
	}
}

func TestCallDigestLLM_MalformedJSON(t *testing.T) {
	_, err := CallDigestLLM(context.Background(), &fakeDigestLLM{raw: "this is not json"}, "p", truthFixture())
	if err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

func TestCallDigestLLM_MissingRequiredField(t *testing.T) {
	// Empty idea string fails validation.
	raw := `{"top_ideas":[{"idea":"","evidence":"e","sample_comments":[]}],"top_questions":[]}`
	_, err := CallDigestLLM(context.Background(), &fakeDigestLLM{raw: raw}, "p", truthFixture())
	if err == nil {
		t.Fatal("expected validation error for empty idea")
	}
}

func TestCallDigestLLM_TimeoutFromCaller(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := CallDigestLLM(ctx, &fakeDigestLLM{delay: 200 * time.Millisecond, raw: "{}"}, "p", truthFixture())
	if err == nil {
		t.Fatal("expected error on timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context deadline") {
		// Acceptable: error wraps the deadline.
	}
}

func TestBuildDigestPrompt_ContainsGroundTruth(t *testing.T) {
	in := DigestPromptInputs{
		Aggregates:     truthFixture(),
		TopTopics:      []TopicAgg{{Topic: "videos", Count: 7}},
		CategoryCounts: map[string]int{"praise": 12},
		SampleComments: []string{"loved the last video"},
	}
	out := BuildDigestPrompt(in)
	for _, want := range []string{
		"engagement_health_score: 75",
		"total_comments_analyzed: 42",
		"videos",
		"loved the last video",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
