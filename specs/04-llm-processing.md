# Spec: LLM Comment Processing

## Overview
Implement an asynchronous batch LLM processing pipeline that classifies comments into 8 categories, extracts topics, computes sentiment, and stores results in the shared `comments` table. Supports configurable providers (OpenAI primary, Anthropic fallback) with automatic failover.

## Requirements
- LLM-01: Classify each comment into 8 categories with confidence ≥ 0.7
- LLM-02: Extract primary topic (≤ 10 words)
- LLM-03: Compute sentiment score (-1.0 to +1.0, 2 decimal precision)
- LLM-04: Generate weekly Insight Digest (see separate Digest spec)
- LLM-05: Toxic/Spam comments excluded from insights by default
- LLM-06: Custom classification prompt per channel (advanced mode, Could have)
- LLM-07: Batch processing in groups of 50 for cost efficiency
- LLM-08: Configurable provider with automatic failover

## Categories
1. `question`
2. `praise`
3. `constructive_criticism`
4. `toxic_criticism`
5. `content_idea`
6. `collaboration_ask`
7. `spam`
8. `other`

## Database Changes

Add to `comments`:
- `llm_provider TEXT` — which provider processed this comment
- `llm_model TEXT` — model version used

Add to `platform_channels`:
- `custom_prompt_prefix TEXT` — max 500 chars, nullable (for LLM-06)

Create processing queue table:
```sql
CREATE TABLE llm_processing_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'processing', 'completed', 'failed'
    comment_ids UUID[] NOT NULL,
    provider TEXT,
    retry_count INT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);
CREATE INDEX idx_llm_jobs_pending ON llm_processing_jobs(status, created_at) WHERE status = 'pending';
```

## LLM Prompt Design

### Classification Prompt (per batch of 50)
```
You are a comment analysis engine for content creators. For each comment below, provide:
1. category: one of [question, praise, constructive_criticism, toxic_criticism, content_idea, collaboration_ask, spam, other]
2. confidence: 0.0 to 1.0
3. topic: the main subject in ≤10 words
4. sentiment: -1.0 (very negative) to +1.0 (very positive), 2 decimals

Return ONLY a JSON array with no markdown formatting:
[
  {"comment_index": 0, "category": "...", "confidence": 0.95, "topic": "...", "sentiment": 0.85},
  ...
]

Comments:
0. [comment text]
1. [comment text]
...
```

- If custom prompt prefix exists for channel, prepend it to the system message
- Batch only comments from the same channel to avoid cross-user data leakage in prompt context

## Batch Processor Architecture

### Background Worker (`internal/llm/processor.go`)

```
Every 10 seconds:
  1. Fetch up to 50 unprocessed comments from DB (processed_at IS NULL)
     - Group by platform_channel_id for batching
  2. For each batch:
     a. Build prompt with comment texts
     b. Call LLM provider
     c. Parse JSON response
     d. Update each comment with category, sentiment, topic, confidence, processed_at
     e. Increment user's comments_used_this_period (per primary tracker or all trackers? Decision needed)
  3. If provider fails >5 times in 60s: switch to fallback provider
```

### Provider Configuration
```go
type LLMProvider string
const (
    ProviderOpenAI LLMProvider = "openai"
    ProviderAnthropic LLMProvider = "anthropic"
)

type LLMClient interface {
    ClassifyBatch(ctx context.Context, comments []Comment) ([]LLMResult, error)
}
```

### OpenAI Implementation
- Model: `gpt-4o`
- API: `chat.completions.create`
- Parse JSON from response
- Handle rate limits (429) with exponential backoff

### Anthropic Fallback
- Model: `claude-3-5-sonnet`
- API: `messages.create`
- Same JSON output format
- Activated when primary returns >5 errors in 60 seconds

### Failover Logic
- Track error count with sliding window (60 seconds)
- On threshold: log provider switch, reset error count
- On success with fallback: continue using fallback until next restart or until primary succeeds manually

### Cost Tracking
- Per batch: estimate tokens = input tokens + output tokens
- Per comment cost = batch_cost / batch_size
- Expose in billing dashboard (see Billing spec)

## Quota Enforcement

Before processing a batch:
1. Check if any tracker of this channel has exceeded quota
2. If all trackers are over quota: skip batch, mark comments as `deferred` or leave unprocessed
3. If at least one tracker has quota: process and increment `comments_used_this_period` for that tracker (the one with highest plan or oldest tracking)

Actually, per the PRD, comments are shared and processed once. The cost should be attributed. For v1:
- Increment `comments_used_this_period` for the primary token holder (the user whose token is used for ingestion)
- If primary is over quota but other trackers have quota, still process and attribute to primary (they're the one providing API access)

Alternative (simpler): increment for all active trackers proportionally, or just track at channel level and bill the primary. For MVP, attribute to primary token holder.

## Backend Implementation

### Update Files
- `internal/llm/client.go` — implement OpenAI and Anthropic clients
- `internal/llm/processor.go` — batch processing worker
- `internal/llm/prompts.go` — classification prompt templates

### New Methods in DBQuery
```go
GetUnprocessedComments(ctx context.Context, limit int) ([]*Comment, error) // exists, ensure groups by channel
UpdateCommentLLMData(ctx context.Context, id, category, topic string, sentiment, confidence float64) error // exists
IncrementCommentsUsed(ctx context.Context, userID string, count int) error
GetPrimaryTracker(ctx context.Context, platformChannelID string) (*User, error)
```

## Testing Checklist
- [ ] Batch of 50 comments is classified correctly
- [ ] Confidence < 0.7 defaults to "other"
- [ ] Sentiment scores are within [-1.0, +1.0]
- [ ] Topics are ≤ 10 words
- [ ] Toxic/spam comments are flagged
- [ ] Provider failover triggers after >5 errors in 60s
- [ ] Quota enforcement pauses processing when exceeded
- [ ] Custom prompt prefix is prepended when set
- [ ] JSON parsing handles edge cases (markdown wrappers, etc.)

## Files to Create/Modify
- `internal/llm/client.go` — implement dual provider support
- `internal/llm/processor.go` — batch worker with queue
- `internal/llm/prompts.go` — prompt engineering
- `internal/db/db.go` — add increment methods
- `internal/db/comments.go` — update LLM data methods
