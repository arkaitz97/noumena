package llm

import (
	"encoding/json"
	"sort"

	"noumena/internal/db"
)

// DigestContent is the canonical JSON shape persisted in
// weekly_digests.content. The LLM is asked to produce the qualitative fields
// (top_ideas, top_questions, top_actionable_insights); the numeric fields are computed in Go and
// authoritatively overwritten before persistence.
type DigestContent struct {
	TopIdeas              []DigestIdea           `json:"top_ideas"`
	TopQuestions          []DigestQuestion       `json:"top_questions"`
	TopActionableInsights []ActionableInsightAgg `json:"top_actionable_insights"`
	EngagementHealthScore int                    `json:"engagement_health_score"`
	HealthScoreBreakdown  HealthScoreBreakdown   `json:"health_score_breakdown"`
	TotalCommentsAnalyzed int                    `json:"total_comments_analyzed"`
	FilteredComments      int                    `json:"filtered_comments"`
}

// ActionableInsightAgg is an aggregated actionable insight for the digest.
type ActionableInsightAgg struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Count       int    `json:"count"`
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

// HealthScoreBreakdown decomposes engagement_health_score into its 2 equal-
// weight components, each in [0, 100].
type HealthScoreBreakdown struct {
	ResponseRate    int `json:"response_rate"`
	EngagementDepth int `json:"engagement_depth"`
}

// TopicAgg is a (topic, count) tuple emitted by computeTopTopics.
type TopicAgg struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

// computeTopActionableInsights extracts and aggregates actionable insights from comments.
func computeTopActionableInsights(comments []*db.Comment, n int) []ActionableInsightAgg {
	if len(comments) == 0 {
		return nil
	}
	type insightKey struct {
		Type        string
		Description string
	}
	counts := map[insightKey]int{}
	for _, c := range comments {
		if len(c.ActionableInsights) == 0 {
			continue
		}
		var insights []Insight
		if err := json.Unmarshal(c.ActionableInsights, &insights); err != nil {
			continue
		}
		for _, ins := range insights {
			key := insightKey{Type: ins.Type, Description: ins.Description}
			counts[key]++
		}
	}
	out := make([]ActionableInsightAgg, 0, len(counts))
	for k, count := range counts {
		out = append(out, ActionableInsightAgg{Type: k.Type, Description: k.Description, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Description < out[j].Description
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
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
	v := (b.ResponseRate + b.EngagementDepth) / 2
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
// by confidence (so high-confidence classifications win) with recency as the tiebreaker.
func selectSampleComments(comments []*db.Comment, n int) []string {
	if n <= 0 || len(comments) == 0 {
		return nil
	}
	type ranked struct {
		body       string
		confidence float64
		hasConf    bool
	}
	scored := make([]ranked, 0, len(comments))
	for _, c := range comments {
		r := ranked{body: c.Body}
		if c.Confidence != nil {
			r.confidence = *c.Confidence
			r.hasConf = true
		}
		scored = append(scored, r)
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].hasConf != scored[j].hasConf {
			return scored[i].hasConf
		}
		if scored[i].confidence != scored[j].confidence {
			return scored[i].confidence > scored[j].confidence
		}
		return true
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
