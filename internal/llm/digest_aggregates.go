package llm

import (
	"math"
	"sort"
	"time"

	"noumena/internal/db"
)

// DigestContent is the canonical JSON shape persisted in
// weekly_digests.content. The LLM is asked to produce the qualitative fields
// (top_ideas, top_questions); the numeric fields are computed in Go and
// authoritatively overwritten before persistence.
type DigestContent struct {
	TopIdeas              []DigestIdea           `json:"top_ideas"`
	TopQuestions          []DigestQuestion       `json:"top_questions"`
	SentimentTrend        SentimentTrend         `json:"sentiment_trend"`
	EngagementHealthScore int                    `json:"engagement_health_score"`
	HealthScoreBreakdown  HealthScoreBreakdown   `json:"health_score_breakdown"`
	TotalCommentsAnalyzed int                    `json:"total_comments_analyzed"`
	FilteredComments      int                    `json:"filtered_comments"`
}

// DigestIdea is one entry in the LLM-synthesised top_ideas list.
type DigestIdea struct {
	Idea           string   `json:"idea"`
	Evidence       string   `json:"evidence"`
	SampleComments []string `json:"sample_comments"`
}

// DigestQuestion is one entry in the LLM-synthesised top_questions list.
type DigestQuestion struct {
	Question   string `json:"question"`
	Count      int    `json:"count"`
	Unanswered bool   `json:"unanswered"`
}

// SentimentTrend captures the current-week sentiment baseline plus the
// week-over-week delta. Direction values: improving | stable | declining.
type SentimentTrend struct {
	CurrentAvg float64 `json:"current_avg"`
	ChangePct  float64 `json:"change_pct"`
	Direction  string  `json:"direction"`
}

// HealthScoreBreakdown decomposes engagement_health_score into its 3 equal-
// weight components, each in [0, 100].
type HealthScoreBreakdown struct {
	ResponseRate     int `json:"response_rate"`
	SentimentQuality int `json:"sentiment_quality"`
	EngagementDepth  int `json:"engagement_depth"`
}

// TopicAgg is a (topic, count) tuple emitted by computeTopTopics.
type TopicAgg struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

// computeSentimentAverage returns the arithmetic mean of all non-NULL
// sentiment_score values across the comments. Returns 0 for an empty input
// (matches Spec 06 baseline-zero scenario).
func computeSentimentAverage(comments []*db.Comment) float64 {
	if len(comments) == 0 {
		return 0
	}
	var sum float64
	var n int
	for _, c := range comments {
		if c.SentimentScore == nil {
			continue
		}
		sum += *c.SentimentScore
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// computeSentimentTrend implements the ±1.0 percentage-point thresholds
// from Spec 06: improving when change_pct > 1.0, declining when < -1.0,
// otherwise stable. When prevAvg == 0 we return ("stable", 0) because the
// baseline is missing or genuinely neutral.
func computeSentimentTrend(currentAvg, prevAvg float64) (direction string, changePct float64) {
	if prevAvg == 0 {
		return "stable", 0
	}
	changePct = ((currentAvg - prevAvg) / math.Abs(prevAvg)) * 100
	switch {
	case changePct > 1.0:
		return "improving", changePct
	case changePct < -1.0:
		return "declining", changePct
	default:
		return "stable", changePct
	}
}

// computeResponseRate is the v1 placeholder for response_rate. The
// canonical input (creator_replied) does not exist yet; per design.md D4
// we substitute an engagement-depth proxy based on volume scaled to a
// configurable baseline. The minimum of 100 caps high-engagement weeks.
func computeResponseRate(comments []*db.Comment) int {
	const expectedBaseline = 100 // tunable; design.md D4 placeholder
	if len(comments) == 0 {
		return 0
	}
	pct := float64(len(comments)) / float64(expectedBaseline) * 100
	if pct > 100 {
		pct = 100
	}
	return int(pct)
}

// computeSentimentQuality maps the [-1, 1] sentiment average onto the
// [0, 100] integer range: ((avg+1)/2)*100. Spec 06 D4 formula.
func computeSentimentQuality(avg float64) int {
	v := ((avg + 1) / 2) * 100
	if v < 0 {
		v = 0
	} else if v > 100 {
		v = 100
	}
	return int(v)
}

// computeEngagementDepth combines mean comment length (length component,
// capped at 500 chars → 100) and topic diversity (distinct topics /
// total ideas, scaled to 100), then averages them. The cap and target are
// v1 heuristics; tune as data accumulates.
func computeEngagementDepth(comments []*db.Comment) int {
	if len(comments) == 0 {
		return 0
	}
	const lengthCap = 500.0 // chars → maps to 100
	const topicTarget = 10.0
	var totalLen int
	topics := map[string]struct{}{}
	for _, c := range comments {
		totalLen += len(c.Body)
		if c.Topic != nil && *c.Topic != "" {
			topics[*c.Topic] = struct{}{}
		}
	}
	avgLen := float64(totalLen) / float64(len(comments))
	lengthScore := (avgLen / lengthCap) * 100
	if lengthScore > 100 {
		lengthScore = 100
	}
	topicScore := (float64(len(topics)) / topicTarget) * 100
	if topicScore > 100 {
		topicScore = 100
	}
	return int((lengthScore + topicScore) / 2)
}

// computeHealthScore collapses the breakdown to a single 0–100 integer via
// the equal-weight mean. Spec 06: engagement_health_score range.
func computeHealthScore(b HealthScoreBreakdown) int {
	v := (b.ResponseRate + b.SentimentQuality + b.EngagementDepth) / 3
	if v < 0 {
		v = 0
	} else if v > 100 {
		v = 100
	}
	return v
}

// computeTopTopics returns the n most-frequent comment topics by raw count,
// breaking ties alphabetically (stable order for snapshot tests).
func computeTopTopics(comments []*db.Comment, n int) []TopicAgg {
	if n <= 0 {
		return nil
	}
	counts := map[string]int{}
	for _, c := range comments {
		if c.Topic != nil && *c.Topic != "" {
			counts[*c.Topic]++
		}
	}
	out := make([]TopicAgg, 0, len(counts))
	for t, c := range counts {
		out = append(out, TopicAgg{Topic: t, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Topic < out[j].Topic
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// computeCategoryDistribution buckets comments by category. NULL categories
// are skipped (caller can derive an "unclassified" count from the totals if
// needed).
func computeCategoryDistribution(comments []*db.Comment) map[string]int {
	out := map[string]int{}
	for _, c := range comments {
		if c.Category == nil || *c.Category == "" {
			continue
		}
		out[*c.Category]++
	}
	return out
}

// selectSampleComments returns up to n high-signal comment bodies, ranked
// by |sentiment| (so strong opinions win) with recency as the tiebreaker.
// Comments without a sentiment score sort last.
func selectSampleComments(comments []*db.Comment, n int) []string {
	if n <= 0 || len(comments) == 0 {
		return nil
	}
	type ranked struct {
		body      string
		absSent   float64
		hasSent   bool
		postedAt  time.Time
	}
	scored := make([]ranked, 0, len(comments))
	for _, c := range comments {
		r := ranked{body: c.Body, postedAt: c.PostedAt}
		if c.SentimentScore != nil {
			r.absSent = math.Abs(*c.SentimentScore)
			r.hasSent = true
		}
		scored = append(scored, r)
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].hasSent != scored[j].hasSent {
			return scored[i].hasSent
		}
		if scored[i].absSent != scored[j].absSent {
			return scored[i].absSent > scored[j].absSent
		}
		return scored[i].postedAt.After(scored[j].postedAt)
	})
	if len(scored) > n {
		scored = scored[:n]
	}
	out := make([]string, 0, len(scored))
	for _, r := range scored {
		out = append(out, r.body)
	}
	return out
}
