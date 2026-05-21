package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// digestCallTimeout is the per-call context timeout (Spec 06: per-call
// timeout 90s). Applied via context.WithTimeout in CallDigestLLM regardless
// of the parent context's deadline so a slow LLM cannot hold the worker.
const digestCallTimeout = 90 * time.Second

// digestSampleLimit is the maximum number of comment bodies fed into the
// prompt as evidence material. The aggregates already carry the numeric
// summary; the samples exist only for the LLM to ground its qualitative
// synthesis.
const digestSampleLimit = 30

// DigestPromptInputs is the structured payload BuildDigestPrompt converts
// into the LLM-facing string. Keeping the inputs as a struct (rather than
// passing 10 args) makes unit tests for the prompt builder readable.
type DigestPromptInputs struct {
	Aggregates     DigestContent
	TopTopics      []TopicAgg
	CategoryCounts map[string]int
	SampleComments []string
}

// BuildDigestPrompt produces the LLM prompt described in Spec 06. Numeric
// facts are injected as ground truth; the LLM is asked to ECHO them back
// (we still authoritatively overwrite on parse) and to focus on qualitative
// synthesis.
func BuildDigestPrompt(in DigestPromptInputs) string {
	var b strings.Builder
	b.WriteString("You are an expert content strategist analyzing a creator's audience comments from the past week.\n\n")
	b.WriteString("GROUND TRUTH (computed by the application — do NOT recalculate; ECHO these values verbatim in your response):\n")
	fmt.Fprintf(&b, "  total_comments_analyzed: %d\n", in.Aggregates.TotalCommentsAnalyzed)
	fmt.Fprintf(&b, "  filtered_comments: %d\n", in.Aggregates.FilteredComments)
	fmt.Fprintf(&b, "  engagement_health_score: %d\n", in.Aggregates.EngagementHealthScore)
	fmt.Fprintf(&b, "  health_score_breakdown: response_rate=%d engagement_depth=%d\n",
		in.Aggregates.HealthScoreBreakdown.ResponseRate,
		in.Aggregates.HealthScoreBreakdown.EngagementDepth)
	if len(in.Aggregates.TopActionableInsights) > 0 {
		b.WriteString("  top_actionable_insights:\n")
		for _, ins := range in.Aggregates.TopActionableInsights {
			fmt.Fprintf(&b, "    - [%s] %s (count: %d)\n", ins.Type, ins.Description, ins.Count)
		}
	}

	if len(in.TopTopics) > 0 {
		b.WriteString("  top_topics:\n")
		for _, t := range in.TopTopics {
			fmt.Fprintf(&b, "    - %s (%d)\n", t.Topic, t.Count)
		}
	}
	if len(in.CategoryCounts) > 0 {
		b.WriteString("  category_distribution:\n")
		for k, v := range in.CategoryCounts {
			fmt.Fprintf(&b, "    %s: %d\n", k, v)
		}
	}

	if len(in.SampleComments) > 0 {
		b.WriteString("\nSAMPLE COMMENTS (high-signal subset for grounding only):\n")
		for i, s := range in.SampleComments {
			s = strings.ReplaceAll(s, "\n", " ")
			fmt.Fprintf(&b, "  [%d] %s\n", i, s)
		}
	}

	b.WriteString(`
TASK: Produce a JSON object with EXACTLY this shape:
{
  "top_ideas": [{"idea": "...", "evidence": "...", "sample_comments": ["...","..."]}],
  "top_questions": [{"question": "...", "count": <int>=1, "unanswered": <bool>}],
  "top_actionable_insights": [{"type": "video_idea", "description": "...", "count": <int>}],
  "engagement_health_score": <int 0-100>,
  "health_score_breakdown": {"response_rate": <int 0-100>, "engagement_depth": <int 0-100>},
  "total_comments_analyzed": <int>,
  "filtered_comments": <int>
}

RULES:
- top_ideas: at most 5 entries; each must have non-empty idea + evidence; sample_comments quotes drawn from the SAMPLE COMMENTS section above.
- top_questions: at most 5 entries; count >= 1; unanswered is your best estimate.
- top_actionable_insights: at most 5 entries; type must be one of: video_idea, editing_tip, feature_request, collaboration_ask, improvement_suggestion, content_format_request, other.
- engagement_health_score, health_score_breakdown, totals: ECHO the GROUND TRUTH values exactly.
- Return ONLY the JSON object. No markdown fences, no prose before or after.
`)
	return b.String()
}

// CallDigestLLM invokes the OpenCode client with a 90-second timeout,
// parses the JSON response, validates required fields, truncates top_ideas /
// top_questions to 5 entries, and authoritatively overwrites the numeric
// fields with the application-computed values from `truth`. On any error
// the digest is treated as a failed attempt by the caller's retry loop.
func CallDigestLLM(ctx context.Context, client *opencodeClient, prompt string, truth DigestContent) (DigestContent, error) {
	cctx, cancel := context.WithTimeout(ctx, digestCallTimeout)
	defer cancel()

	raw, err := client.GenerateDigestRaw(cctx, prompt)
	if err != nil {
		return DigestContent{}, fmt.Errorf("digest LLM call failed: %w", err)
	}

	parsed, err := parseDigestContent(raw)
	if err != nil {
		return DigestContent{}, err
	}

	// Truncate qualitative arrays to the documented max (Spec 06: at most 5).
	if len(parsed.TopIdeas) > 5 {
		parsed.TopIdeas = parsed.TopIdeas[:5]
	}
	if len(parsed.TopQuestions) > 5 {
		parsed.TopQuestions = parsed.TopQuestions[:5]
	}

	// Authoritatively overwrite numeric ground truth (Spec 06: numeric fields
	// authoritative over LLM output). The LLM-supplied numbers are advisory
	// only; we never trust them for persistence.
	parsed.TopActionableInsights = truth.TopActionableInsights
	parsed.EngagementHealthScore = truth.EngagementHealthScore
	parsed.HealthScoreBreakdown = truth.HealthScoreBreakdown
	parsed.TotalCommentsAnalyzed = truth.TotalCommentsAnalyzed
	parsed.FilteredComments = truth.FilteredComments

	return parsed, nil
}

// parseDigestContent strips optional markdown fences, locates the first JSON
// object, decodes it tolerantly, then validates that the qualitative fields
// are well-formed (Spec 06 EC: "schema validation failure counts as an
// attempt failure").
func parseDigestContent(raw string) (DigestContent, error) {
	s := stripMarkdownFences(raw)
	// Find the first '{' and the last '}' and decode that span. JSON-mode
	// LLMs typically emit a clean object, but we are defensive.
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return DigestContent{}, fmt.Errorf("no JSON object found in response")
	}
	jsonStr := s[start : end+1]

	var dc DigestContent
	if err := json.Unmarshal([]byte(jsonStr), &dc); err != nil {
		return DigestContent{}, fmt.Errorf("digest JSON unmarshal: %w", err)
	}

	// Required-field validation. Empty arrays are OK (e.g. a quiet week with
	// no questions); we only reject genuinely malformed entries that lack
	// the documented keys. Strict required-field checks would over-reject
	// legitimately sparse weeks, so we only insist on basic well-formedness.
	for i, idea := range dc.TopIdeas {
		if idea.Idea == "" {
			return DigestContent{}, fmt.Errorf("top_ideas[%d].idea is empty", i)
		}
	}
	for i, q := range dc.TopQuestions {
		if q.Question == "" {
			return DigestContent{}, fmt.Errorf("top_questions[%d].question is empty", i)
		}
		if q.Count < 1 {
			return DigestContent{}, fmt.Errorf("top_questions[%d].count must be >= 1", i)
		}
	}
	return dc, nil
}
