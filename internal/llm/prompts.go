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

// SystemPrompt is the system message the LLM receives. It enforces the
// closed taxonomy, the JSON-only output, the sentiment range and the topic
// length cap. The prompt explicitly forbids markdown wrappers, but the parser
// is defensive and tolerates them anyway (see ParseClassificationResponse).
const SystemPrompt = `You are a community-management classifier for a creator's audience comments.

For each comment in the user message, return ONE JSON object with exactly these fields:
- comment_index (integer, the zero-based index of the comment in the user message)
- category (string, EXACTLY one of: question, praise, constructive_criticism, toxic_criticism, content_idea, collaboration_ask, spam, other)
- confidence (number in [0.0, 1.0], your confidence in the category)
- topic (string, at most 10 whitespace-separated words describing the primary subject)
- sentiment (number in [-1.0, 1.0], rounded to two decimals)

Return ONLY a single JSON array containing one object per input comment, in the same order. Do NOT wrap the array in markdown code fences. Do NOT include any prose before or after the array. Do NOT add comments inside the JSON.`

// LLMResult is the parsed-and-validated classification for a single comment.
// CommentIndex is the zero-based position in the prompt's enumerated list.
type LLMResult struct {
	CommentIndex int     `json:"comment_index"`
	Category     string  `json:"category"`
	Confidence   float64 `json:"confidence"`
	Topic        string  `json:"topic"`
	Sentiment    float64 `json:"sentiment"`
}

// PromptComment is the minimal shape required to build a classification
// prompt. It is decoupled from db.Comment so the llm package does not import
// db.
type PromptComment struct {
	Body string
}

// maxCommentBytes is the per-comment truncation limit applied at prompt-build
// time to keep total prompt size predictable.
const maxCommentBytes = 2048

// BuildClassificationPrompt produces the (system, user) message pair for a
// single per-channel batch. The user message is a numbered list of comments
// truncated to 2KB each.
func BuildClassificationPrompt(comments []PromptComment) (system string, user string) {
	var b strings.Builder
	b.WriteString("Classify the following comments. Return a JSON array with one object per comment.\n\n")
	for i, c := range comments {
		body := c.Body
		if len(body) > maxCommentBytes {
			body = body[:maxCommentBytes]
		}
		// Escape backticks defensively; the LLM input is plain text.
		body = strings.ReplaceAll(body, "\n", " ")
		fmt.Fprintf(&b, "[%d] %s\n", i, body)
	}
	return SystemPrompt, b.String()
}

// jsonArrayRe extracts the first balanced top-level JSON array from the raw
// response, after fence stripping. Used as a tolerant fallback when the model
// emits trailing prose after the array.
var jsonArrayRe = regexp.MustCompile(`(?s)\[.*\]`)

// stripMarkdownFences removes ```json ... ``` and ``` ... ``` wrappers and
// trims surrounding whitespace.
func stripMarkdownFences(raw string) string {
	s := strings.TrimSpace(raw)
	// ```json ... ```  or  ``` ... ```
	if strings.HasPrefix(s, "```") {
		// Drop the opening fence line.
		if nl := strings.Index(s, "\n"); nl >= 0 {
			s = s[nl+1:]
		} else {
			s = strings.TrimPrefix(s, "```")
		}
		// Drop trailing fence.
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
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
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

// ParseClassificationResponse strips fences, extracts the first balanced JSON
// array, validates each entry against the closed category set and the
// [-1.0, 1.0] sentiment range, rounds sentiment to 2 decimals, and truncates
// topic to 10 whitespace-separated words. The returned slice is in the same
// order the model emitted; callers use CommentIndex to align with input.
func ParseClassificationResponse(raw string, expectedCount int) ([]LLMResult, error) {
	s := stripMarkdownFences(raw)
	jsonStr, ok := extractFirstJSONArray(s)
	if !ok {
		// Last-resort regex fallback (greedy, takes the widest [...] span).
		if m := jsonArrayRe.FindString(s); m != "" {
			jsonStr = m
		} else {
			return nil, fmt.Errorf("no JSON array found in response")
		}
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
		if r.Sentiment < -1.0 || r.Sentiment > 1.0 {
			return nil, fmt.Errorf("sentiment %v out of range at index %d", r.Sentiment, r.CommentIndex)
		}
		// Round sentiment to 2 decimals.
		r.Sentiment = math.Round(r.Sentiment*100) / 100
		// Truncate topic to first 10 whitespace-separated words.
		r.Topic = truncateWords(r.Topic, 10)
		validated = append(validated, r)
	}
	return validated, nil
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
// value verbatim. Returns a new slice; the input is not mutated.
func ApplyConfidenceThreshold(results []LLMResult, threshold float64) []LLMResult {
	out := make([]LLMResult, len(results))
	copy(out, results)
	for i := range out {
		if out[i].Confidence < threshold {
			out[i].Category = CategoryOther
		}
	}
	return out
}

// DigestPrompt is retained from the prior implementation for the weekly
// digest worker; it is unrelated to per-comment classification but lives in
// the same package.
const DigestPrompt = `You are an expert content strategist analyzing a week's worth of comments across a creator's channels.
Given the following JSON array of processed comments, synthesize a weekly insight digest.
Focus on identifying top content ideas asked by the audience and the most common/urgent questions.

Provide the output strictly in the following JSON format:
{
  "top_ideas": ["Idea 1", "Idea 2", "Idea 3", "Idea 4", "Idea 5"],
  "top_questions": ["Question 1", "Question 2", "Question 3", "Question 4", "Question 5"],
  "sentiment_trend": "Improving / Stable / Declining",
  "health_score": 85
}`
