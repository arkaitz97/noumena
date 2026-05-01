# Spec: Insights Dashboard API

## Overview
Build the backend API for the insights dashboard: real-time high-signal comment feed, sentiment trends, filtering, starring, and exports.

## Requirements
- DASH-01: Real-time feed of high-signal comments (Questions, Content Ideas, Collaboration Asks)
- DASH-02: Sentiment trend line (7d, 30d, 90d) per channel and globally
- DASH-03: Filter by category, channel, date range, sentiment score (all combinable, p95 < 2s)
- DASH-04: Star comments for later review
- DASH-05: Toxicity Shield metric (% toxic/spam filtered)
- DASH-06: Export filtered views as CSV/JSON (up to 50K rows, queued for large sets)
- DASH-07: Weekly digest rendered in dashboard and emailed

## Database Changes

No schema changes required (uses existing tables).

Add composite indexes for performance:
```sql
CREATE INDEX idx_comments_channel_processed ON comments(platform_channel_id, processed_at, category);
CREATE INDEX idx_comments_channel_posted ON comments(platform_channel_id, posted_at);
CREATE INDEX idx_comments_user_starred ON user_comment_stars(user_id, starred_at);
```

## API Endpoints

### GET /api/comments/feed
- Query params:
  - `channels` — comma-separated channel IDs (optional)
  - `categories` — comma-separated categories (optional)
  - `sentiment_min` — float, optional
  - `sentiment_max` — float, optional
  - `date_from` — ISO date, optional
  - `date_to` — ISO date, optional
  - `search` — text search in body/topic, optional
  - `limit` — default 20, max 100
  - `offset` — default 0
  - `sort` — `newest` | `sentiment_desc` | `sentiment_asc`
- Returns comments the user has access to (via active or paused tracking relationships)
- Default (no filters): shows high-signal categories (`question`, `content_idea`, `collaboration_ask`, `praise`, `constructive_criticism`)
- Excludes `toxic_criticism` and `spam` unless user explicitly includes them via `include_toxic=true`
- Unprocessed comments (processed_at IS NULL) are shown with `status: "processing"` badge but excluded from default feed
- Response:
```json
{
  "comments": [
    {
      "id": "uuid",
      "platform_channel_id": "uuid",
      "channel_name": "My Channel",
      "author_handle": "@user",
      "body": "...",
      "posted_at": "2026-03-01T12:00:00Z",
      "category": "question",
      "sentiment_score": 0.75,
      "topic": "camera setup",
      "confidence": 0.92,
      "processed_at": "2026-03-01T12:05:00Z",
      "is_starred": true,
      "status": "processed"
    }
  ],
  "total": 142,
  "has_more": true
}
```

### GET /api/comments/saved
- Returns only starred comments for current user
- Supports same filtering as `/feed` (except channel filter is still allowed)
- Sorted by `starred_at DESC`

### POST /api/comments/{id}/star
- Toggle star status for a comment
- If already starred: remove star (DELETE semantics via toggle)
- Alternative: `POST /api/comments/{id}/star` to add, `DELETE /api/comments/{id}/star` to remove
- **Decision needed**: Use toggle or explicit add/remove? Recommend explicit for clarity.
- Let's use explicit: `POST /api/comments/{id}/star` and `DELETE /api/comments/{id}/star`

### GET /api/insights/sentiment
- Query params: `channel_id` (optional, default all tracked channels)
- Returns sentiment aggregates:
```json
{
  "channel_id": "uuid or 'all'",
  "periods": {
    "7d": { "avg": 0.65, "count": 1200, "positive_pct": 72, "negative_pct": 15, "neutral_pct": 13 },
    "30d": { "avg": 0.58, "count": 5400, "positive_pct": 68, "negative_pct": 18, "neutral_pct": 14 },
    "90d": { "avg": 0.61, "count": 15200, "positive_pct": 70, "negative_pct": 16, "neutral_pct": 14 }
  }
}
```
- Only includes processed comments
- Excludes toxic/spam by default

### GET /api/insights/toxicity-shield
- Query params: `channel_id` (optional)
- Returns:
```json
{
  "channel_id": "uuid or 'all'",
  "periods": {
    "7d": { "total_comments": 1200, "filtered_count": 145, "filtered_pct": 12.1 },
    "30d": { "total_comments": 5400, "filtered_count": 890, "filtered_pct": 16.5 }
  }
}
```

### GET /api/insights/trends
- Returns top topics and categories over time
- Query params: `channel_id`, `days` (7, 30, 90)
- Response:
```json
{
  "top_topics": [
    { "topic": "camera setup", "count": 45, "avg_sentiment": 0.82 },
    { "topic": "lighting", "count": 32, "avg_sentiment": 0.71 }
  ],
  "top_categories": [
    { "category": "question", "count": 120 },
    { "category": "content_idea", "count": 45 }
  ]
}
```

### POST /api/comments/export
- Body: same filters as `/feed` + `format` (`csv` | `json`)
- If estimated result count ≤ 50K: generate synchronously, return download URL (expires in 24h)
- If > 50K: enqueue export job, return `{ "job_id": "...", "status": "queued" }`
- Email notification with download link when complete

### GET /api/comments/export/{job_id}/status
- Returns export job status and download URL when complete

### GET /api/insights/digest/latest
- Returns the most recent weekly digest for the user
- Response includes `top_ideas`, `top_questions`, `sentiment_trend`, `health_score`

## Backend Implementation

### Query Builder
Build a dynamic SQL query builder for the comment feed:
```sql
SELECT c.*, pc.display_name as channel_name, EXISTS(
  SELECT 1 FROM user_comment_stars ucs WHERE ucs.user_id = $1 AND ucs.comment_id = c.id
) as is_starred
FROM comments c
JOIN platform_channels pc ON c.platform_channel_id = pc.id
JOIN user_channel_tracking uct ON pc.id = uct.platform_channel_id
WHERE uct.user_id = $1
  AND uct.tracking_status IN ('active', 'paused')
  AND c.processed_at IS NOT NULL
  /* dynamic filters */
ORDER BY c.posted_at DESC
LIMIT $n OFFSET $m;
```

### Performance Optimization
- Use `COUNT(*)` estimate for large tables (or separate count query with timeout)
- Ensure composite indexes are used
- For sentiment aggregation, consider materialized view refreshed hourly:
```sql
CREATE MATERIALIZED VIEW sentiment_aggregates AS
SELECT 
  platform_channel_id,
  DATE_TRUNC('day', posted_at) as day,
  AVG(sentiment_score) as avg_sentiment,
  COUNT(*) as comment_count,
  SUM(CASE WHEN sentiment_score > 0.3 THEN 1 ELSE 0 END)::float / COUNT(*) as positive_pct
FROM comments
WHERE processed_at IS NOT NULL
  AND category NOT IN ('toxic_criticism', 'spam')
GROUP BY platform_channel_id, DATE_TRUNC('day', posted_at);
```

### Export Worker
- For large exports: stream rows from DB, write to temp file, upload to object storage (or serve directly if <50K)
- Since we're VPS/cloud, write to local disk with cleanup after 24h
- Background goroutine processes queued exports

## Frontend Integration
- Feed view uses infinite scroll (offset-based pagination)
- Filters update URL query params for shareability
- Star toggle updates optimistically

## Testing Checklist
- [ ] Feed returns only comments from tracked channels
- [ ] Default feed excludes toxic/spam
- [ ] All filters are combinable and return within 2s
- [ ] Star/unstar works and persists
- [ ] Sentiment aggregates are accurate
- [ ] Toxicity shield metrics calculate correctly
- [ ] Export generates valid CSV/JSON
- [ ] Large export (>50K) is queued and emailed

## Files to Create/Modify
- `internal/api/comment_handlers.go` — extend with all endpoints
- `internal/db/comments.go` — add feed query, aggregation queries
- `internal/db/digests.go` — add latest digest query
- `internal/insights/aggregates.go` — materialized view refresh logic (new package)
- `internal/export/worker.go` — export generation worker (new)
