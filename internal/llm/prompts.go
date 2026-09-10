package llm

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// ValidCategories is the closed set of category values defined by the
// llm-classification-contract spec. Any LLM output not in this set is treated
// as a parse failure for that comment.
var ValidCategories = map[string]struct{}{
	"question":               {},
	"praise":                 {},
	"constructive_criticism": {},
	"toxic_criticism":        {},
	"content_idea":           {},
	"collaboration_ask":      {},
	"spam":                   {},
	"other":                  {},
}

// CategoryOther is the fallback category used when confidence is below the
// configured threshold.
const CategoryOther = "other"

// ValidInsightTypes is the closed set of actionable insight types the LLM
// may emit. Any insight with a type not in this set is a parse failure.
var ValidInsightTypes = map[string]struct{}{
	"video_idea":             {},
	"editing_tip":            {},
	"feature_request":        {},
	"collaboration_ask":      {},
	"improvement_suggestion": {},
	"content_format_request": {},
	"other":                  {},
}

// Insight represents a single actionable insight extracted from a comment.
type Insight struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
}

const maxInsightsPerComment = 5
const maxInsightDescriptionLen = 200

// SystemPrompt is the system message the LLM receives. It enforces the
// closed taxonomy, the JSON-only output, and the actionable_insights format.
// The prompt explicitly forbids markdown wrappers, but the parser
// is defensive and tolerates them anyway (see ParseClassificationResponse).
const SystemPrompt = `You are a community-management classifier for a creator's audience comments.

For each comment in the user message, return ONE JSON object with exactly these fields:
- comment_index (integer, the zero-based index of the comment in the user message)
- category (string, EXACTLY one of: question, praise, constructive_criticism, toxic_criticism, content_idea, collaboration_ask, spam, other)
- confidence (number in [0.0, 1.0], your confidence in the category)
- sentiment (string, EXACTLY one of: negative, neutral, positive)
- topic (string, at most 10 whitespace-separated words describing the primary subject)
- actionable_insights (array of insight objects, may be empty; each has: type (one of: video_idea, editing_tip, feature_request, collaboration_ask, improvement_suggestion, content_format_request, other), description (string, max 200 characters), confidence (number in [0.0, 1.0]))

Return ONLY a single JSON array containing one object per input comment, in the same order. Do NOT wrap the array in markdown code fences. Do NOT include any prose before or after the array. Do NOT add comments inside the JSON. Extract at most 5 actionable_insights per comment.`

// SentimentLabel is the closed-set sentiment value the LLM emits per
// comment. Labels are case-sensitive and defined by the prompt contract.
type SentimentLabel string

const (
	SentimentNegative SentimentLabel = "negative"
	SentimentNeutral  SentimentLabel = "neutral"
	SentimentPositive SentimentLabel = "positive"
)

// UnmarshalJSON accepts the canonical string label or a legacy numeric
// value (older provider fixtures emit numbers). Numeric values carry no
// valid label and collapse to the empty label (score 0 / neutral) rather
// than failing the whole batch; null is tolerated the same way.
func (s *SentimentLabel) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = ""
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = SentimentLabel(str)
		return nil
	}
	var num float64
	if err := json.Unmarshal(b, &num); err == nil {
		*s = ""
		return nil
	}
	return fmt.Errorf("invalid sentiment value %s", b)
}

// LLMResult is the parsed-and-validated classification for a single comment.
// CommentIndex is the zero-based position in the prompt's enumerated list.
// Sentiment is the closed-set label (negative|neutral|positive) the model
// emits; map it to a score with SentimentScore.
type LLMResult struct {
	CommentIndex       int            `json:"comment_index"`
	Category           string         `json:"category"`
	Confidence         float64        `json:"confidence"`
	Topic              string         `json:"topic"`
	ActionableInsights []Insight      `json:"actionable_insights"`
	Sentiment          SentimentLabel `json:"sentiment"`
}

// sentimentScores maps sentiment labels to aggregation scores; missing or
// unknown labels default to neutral (0) so a single malformed field never
// fails the whole batch, mirroring the tolerance applied to category parsing.
var sentimentScores = map[SentimentLabel]int{
	SentimentNegative: -1,
	SentimentPositive: 1,
}

func SentimentScore(s SentimentLabel) int {
	return sentimentScores[s]
}

// PromptComment is the minimal shape required to build a classification
// prompt. It is decoupled from db.Comment so the llm package does not import
// db.
type PromptComment struct {
	Body          string
	ParentContext string
	CustomPrompt  string
}

func BuildClassificationPrompt(comments []PromptComment) (system string, user string) {
	var customPrompt string
	if len(comments) > 0 {
		customPrompt = comments[0].CustomPrompt
	}
	var b strings.Builder
	if customPrompt != "" {
		b.WriteString("Additional context for classification: ")
		b.WriteString(customPrompt)
		b.WriteString("\n\n")
	}
	b.WriteString("Classify the following comments. Return a JSON array with one object per comment.\n\n")
	for i, c := range comments {
		b.WriteString(promptCommentLine(i, c.Body, c.ParentContext))
	}
	return SystemPrompt, b.String()
}

// promptCommentLine renders one numbered comment line. Replies inline a
// flattened, 200-char excerpt of the parent context.
func promptCommentLine(i int, body, parentContext string) string {
	body = oneLine(truncate(body, maxCommentBytes))
	if parentContext == "" {
		return fmt.Sprintf("[%d] %s\n", i, body)
	}
	return fmt.Sprintf("[%d] (reply to: %s) %s\n", i, oneLine(clipEllipsis(parentContext, 200)), body)
}

func clipEllipsis(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func oneLine(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

// maxCommentBytes is the per-comment truncation limit applied at prompt-build
// time to keep total prompt size predictable.
const maxCommentBytes = 2048

// jsonArrayRe extracts the first balanced top-level JSON array from the raw
// response, after fence stripping. Used as a tolerant fallback when the model
// emits trailing prose after the array.
var jsonArrayRe = regexp.MustCompile(`(?s)\[.*\]`)

// stripMarkdownFences removes ```json ... ``` and ``` ... ``` wrappers and
// trims surrounding whitespace.
func stripMarkdownFences(raw string) string {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = dropOpenFence(s)
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx] // Drop trailing fence.
	}
	return strings.TrimSpace(s)
}

// dropOpenFence removes the opening fence line (or prefix, when there is no
// newline at all).
func dropOpenFence(s string) string {
	if nl := strings.Index(s, "\n"); nl >= 0 {
		return s[nl+1:]
	}
	return strings.TrimPrefix(s, "```")
}

// extractFirstJSONArray returns the substring from the first '[' to the
// matching balanced ']' (string-aware), allowing trailing prose after.
func extractFirstJSONArray(s string) (string, bool) {
	start := strings.Index(s, "[")
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '[' {
			depth++
		}
		if ch != ']' {
			continue
		}
		depth--
		if depth == 0 {
			return s[start : i+1], true
		}
	}
	return "", false
}

// ParseClassificationResponse strips fences, extracts the first balanced JSON
// array, validates each entry against the closed category set and the
// actionable_insights format, rounds confidence to 2 decimals, and truncates
// topic to 10 whitespace-separated words. The returned slice is in the same
// order the model emitted; callers use CommentIndex to align with input.
func ParseClassificationResponse(raw string, expectedCount int) ([]LLMResult, error) {
	s := stripMarkdownFences(raw)
	jsonStr, ok := extractFirstJSONArray(s)
	if !ok {
		// Last-resort regex fallback (greedy, takes the widest [...] span).
		m := jsonArrayRe.FindString(s)
		if m == "" {
			return nil, fmt.Errorf("no JSON array found in response")
		}
		jsonStr = m
	}

	var rawResults []LLMResult
	if err := json.Unmarshal([]byte(jsonStr), &rawResults); err != nil {
		return nil, fmt.Errorf("unmarshal results: %w", err)
	}

	if len(rawResults) != expectedCount {
		return nil, fmt.Errorf("expected %d results, got %d", expectedCount, len(rawResults))
	}

	validated := make([]LLMResult, 0, len(rawResults))
	for _, r := range rawResults {
		if _, ok := ValidCategories[r.Category]; !ok {
			return nil, fmt.Errorf("invalid category %q at index %d", r.Category, r.CommentIndex)
		}
		// Round confidence to 2 decimals.
		r.Confidence = math.Round(r.Confidence*100) / 100
		// Truncate topic to first 10 whitespace-separated words.
		r.Topic = truncateWords(r.Topic, 10)
		// Validate and filter actionable_insights.
		r.ActionableInsights = validateInsights(r.ActionableInsights)
		validated = append(validated, r)
	}
	return validated, nil
}

// validateInsights filters and validates the actionable_insights array:
// - enforces max 5 insights
// - validates type against ValidInsightTypes
// - truncates description to 200 chars
// - rounds confidence to 2 decimals
// - returns only insights with confidence >= 0.0
func validateInsights(insights []Insight) []Insight {
	if len(insights) == 0 {
		return []Insight{}
	}
	validated := make([]Insight, 0, len(insights))
	for _, ins := range insights {
		if _, ok := ValidInsightTypes[ins.Type]; !ok {
			continue
		}
		if ins.Confidence < 0.0 || ins.Confidence > 1.0 {
			continue
		}
		ins.Confidence = math.Round(ins.Confidence*100) / 100
		if len(ins.Description) > maxInsightDescriptionLen {
			ins.Description = ins.Description[:maxInsightDescriptionLen-3] + "..."
		}
		validated = append(validated, ins)
		if len(validated) >= maxInsightsPerComment {
			break
		}
	}
	return validated
}

func truncateWords(s string, n int) string {
	fields := strings.Fields(s)
	if len(fields) <= n {
		return strings.Join(fields, " ")
	}
	return strings.Join(fields[:n], " ")
}

// ApplyConfidenceThreshold downgrades any result whose confidence is below
// `threshold` to category="other" while preserving the original confidence
// value verbatim. Also filters actionable_insights to only include those
// with confidence >= threshold. Returns a new slice; the input is not mutated.
func ApplyConfidenceThreshold(results []LLMResult, threshold float64) []LLMResult {
	out := make([]LLMResult, len(results))
	copy(out, results)
	for i := range out {
		if out[i].Confidence < threshold {
			out[i].Category = CategoryOther
		}
		// Filter insights below threshold.
		filtered := make([]Insight, 0, len(out[i].ActionableInsights))
		for _, ins := range out[i].ActionableInsights {
			if ins.Confidence >= threshold {
				filtered = append(filtered, ins)
			}
		}
		out[i].ActionableInsights = filtered
	}
	return out
}

// DigestPrompt is retained from the prior implementation for the weekly
// digest worker; it is unrelated to per-comment classification but lives in
// the same package.
const DigestPrompt = `You are an expert content strategist analyzing a week's worth of comments across a creator's channels.
Given the following JSON array of processed comments, synthesize a weekly insight digest.
Focus on identifying top content ideas asked by the audience, the most common/urgent questions, and actionable insights from comments.

Provide the output strictly in the following JSON format:
{
  "top_ideas": ["Idea 1", "Idea 2", "Idea 3", "Idea 4", "Idea 5"],
  "top_questions": ["Question 1", "Question 2", "Question 3", "Question 4", "Question 5"],
  "top_actionable_insights": [{"type": "video_idea", "description": "...", "count": 3}, {"type": "editing_tip", "description": "...", "count": 2}],
  "health_score": 85
}`
