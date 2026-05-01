# Spec: Weekly Digest Generation

## Overview
Implement an automated weekly digest generation system that runs every Monday at 08:00 UTC, analyzes the past 7 days of processed comments per user, and generates a structured insight summary using an LLM call. The digest is stored, rendered in the dashboard, and optionally emailed.

## Requirements
- LLM-04: Weekly Insight Digest generated every Monday 08:00 UTC
- DASH-07: Digest rendered in dashboard and optionally emailed
- EC-2.1: Skip generation if < 10 processed comments in last 7 days
- EC-2.2: Retry LLM up to 3 times over 2 hours; mark deferred if all fail
- EC-2.3: Only include comments from channels active at digest generation time

## Database Changes

Existing `weekly_digests` table is sufficient. Add index:
```sql
CREATE INDEX idx_weekly_digests_user_week ON weekly_digests(user_id, week_start_date);
```

Add to `users`:
- `digest_email_opt_in BOOLEAN NOT NULL DEFAULT true`
- `digest_email_sent_at TIMESTAMPTZ` — last successful email

## Digest Content Schema

```json
{
  "top_ideas": [
    { "idea": "Make a video about X", "evidence": "3 comments mentioned this", "sample_comments": ["..."] }
  ],
  "top_questions": [
    { "question": "What camera do you use?", "count": 12, "unanswered": true }
  ],
  "sentiment_trend": {
    "direction": "improving" | "declining" | "stable",
    "change_pct": 5.2,
    "current_avg": 0.72
  },
  "engagement_health_score": 78,
  "health_score_breakdown": {
    "response_rate": 85,
    "sentiment_quality": 72,
    "engagement_depth": 77
  },
  "total_comments_analyzed": 1240,
  "filtered_comments": 180
}
```

## API Endpoints

### GET /api/digests
- Query params: `limit` (default 10), `offset`
- Returns paginated list of weekly digests for current user
- Sorted by `week_start_date DESC`

### GET /api/digests/latest
- Returns the most recent generated digest
- If no digest exists: 404 (or empty object with message)

### PATCH /api/user/preferences
- Body: `{ "digest_email_opt_in": boolean }`
- Updates user's email preference for digest delivery

## Digest Generation Pipeline

### Cron Trigger
- Use Go's `time.Ticker` or cron library to trigger every Monday at 08:00 UTC
- Alternatively, a background worker that checks every minute: `if now.Weekday() == Monday && now.Hour() == 8 && now.Minute() == 0`

### Generation Job
For each active user (has at least one active tracking relationship):

1. **Fetch comments**: 
   - Get all processed comments from last 7 days (since last Monday 00:00 UTC)
   - Filter to channels where `tracking_status = 'active'` at generation time
   - Exclude `toxic_criticism` and `spam`
   - Skip if count < 10 → mark status `skipped_insufficient_data`

2. **Compute aggregates**:
   - Sentiment trend: compare current week avg vs previous week avg
   - Engagement health score: composite of response rate, sentiment quality, engagement depth
   - Top topics by frequency
   - Category distribution

3. **LLM summarization**:
   - Build prompt with aggregated data + sample comments
   - Request structured JSON output (top 5 ideas, top 5 questions)
   - Retry up to 3 times with exponential backoff (5 min, 15 min, 30 min)
   - On success: store JSON in `weekly_digests.content`
   - On failure after retries: status = `deferred`, send delay notification email

4. **Email delivery** (if opted in):
   - Use Resend to send digest email with HTML rendering
   - Update `emailed_at` and `digest_email_sent_at`
   - Email template includes summary + CTA to view full dashboard

### LLM Prompt for Digest
```
You are a content strategist analyzing audience comments for a creator.

Here is a summary of comments from the past week:
- Total analyzed: {count}
- Average sentiment: {avg} (previous week: {prev_avg})
- Top topics: {topics}
- Category breakdown: {breakdown}

Sample high-signal comments:
{sample_comments}

Generate a JSON object with:
1. top_ideas: array of top 5 content ideas with evidence and sample comments
2. top_questions: array of top 5 unanswered questions with count
3. sentiment_trend: { direction, change_pct, current_avg }
4. engagement_health_score: 0-100 based on volume, sentiment, diversity of feedback
5. health_score_breakdown: { response_rate, sentiment_quality, engagement_depth }

Be concise and actionable. The creator should know exactly what to create next.
```

## Backend Implementation

### Update Files
- `internal/llm/digest_worker.go` — existing file, extend with full logic
- `internal/email/client.go` — add digest email template
- `internal/db/digests.go` — add digest queries

### New Methods in DBQuery
```go
GetUsersForDigest(ctx context.Context) ([]*User, error) // exists
GetRecentProcessedCommentsForUser(ctx context.Context, userID string, since time.Time) ([]*Comment, error) // exists
InsertWeeklyDigest(ctx context.Context, digest *WeeklyDigest) error // exists
GetPreviousWeekDigest(ctx context.Context, userID string, weekStart time.Time) (*WeeklyDigest, error)
GetLatestDigest(ctx context.Context, userID string) (*WeeklyDigest, error)
UpdateDigestStatus(ctx context.Context, digestID, status string) error
```

## Edge Cases
- **EC-2.1 (< 10 comments)**: Status = `skipped_insufficient_data`. No email sent. Dashboard shows message.
- **EC-2.2 (LLM unavailable)**: Retry 3 times over 2 hours. If all fail: status = `deferred`, email user about delay.
- **EC-2.3 (Channel removed mid-week)**: Only include channels active at generation time.
- **No previous week for comparison**: Sentiment trend direction = `stable`, change_pct = 0.

## Testing Checklist
- [ ] Digest generates every Monday at 08:00 UTC
- [ ] Skips users with < 10 comments
- [ ] LLM retry logic works (3 retries)
- [ ] Deferred status set and delay email sent on persistent failure
- [ ] Only active channels included
- [ ] Email sent to opted-in users
- [ ] Dashboard displays latest digest correctly
- [ ] Previous week comparison calculates correctly

## Files to Create/Modify
- `internal/llm/digest_worker.go` — implement full generation pipeline
- `internal/db/digests.go` — add query methods
- `internal/email/client.go` — add digest HTML email template
- `internal/api/comment_handlers.go` or new `internal/api/digest_handlers.go` — add digest endpoints
