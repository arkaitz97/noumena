## Why

Sentiment scores (-1.0 to +1.0) provide little actionable value for creators trying to grow their channels. Creators need concrete, extractable feedback from their audience — new video ideas, editing suggestions, collaboration opportunities, and improvement tips — not abstract sentiment numbers. This change pivots the LLM analysis from sentiment classification to actionable insight extraction.

## What Changes

- **Remove** sentiment_score from LLM classification output, database schema, API responses, and frontend visualizations
- **Add** actionable_insights field to LLM classification — a structured array of extractable suggestions (video ideas, editing tips, feature requests, collaboration asks) per comment
- **Replace** sentiment aggregate endpoints with actionable insights aggregation (top ideas, trending suggestions, insight frequency)
- **Update** weekly digest to surface actionable insights instead of sentiment trends
- **Update** frontend dashboard to display actionable insights panels instead of sentiment charts
- **BREAKING**: `sentiment_score` column removed from `comments` table; `GET /api/insights/sentiment` endpoint removed; feed filter `sentiment_min`/`sentiment_max` removed

## Capabilities

### New Capabilities
- `actionable-insights-extraction`: LLM extracts structured actionable insights from comments (video ideas, editing tips, feature requests, collaboration suggestions). Each comment can yield zero or more insights with type, description, and confidence.
- `actionable-insights-aggregation`: API endpoints that aggregate and surface top actionable insights across channels and time windows (top video ideas, trending editing tips, most-requested features).

### Modified Capabilities
- `llm-processing`: Classification contract removes `sentiment` field and adds `actionable_insights` array. Prompt changes from sentiment scoring to insight extraction.
- `insights-api`: Removes sentiment aggregate endpoint and feed sentiment filters. Adds actionable insights listing and aggregation endpoints.
- `weekly-digest`: Digest content replaces `sentiment_trend` with `top_actionable_insights` section.
- `frontend-dashboard`: Removes sentiment trend charts and sentiment-based feed filters. Adds actionable insights panels and insight-type filtering.

## Impact

- **Database**: `comments.sentiment_score` column dropped; `comments.actionable_insights` JSONB column added; sentiment-related indexes removed; new GIN index on actionable_insights
- **Backend (Go)**: `LLMResult` struct changed; `prompts.go` system prompt rewritten; `processor.go` persistence updated; `comments_feed_db.go` sentiment aggregates replaced; new insights aggregation queries; `insights_handlers.go` new endpoints
- **Frontend (Vue 3)**: Sentiment chart components removed; new actionable insights components added; feed filter UI updated; insights dashboard redesigned
- **API**: `GET /api/insights/sentiment` removed; `GET /api/insights/actionable-insights` added; feed query params `sentiment_min`/`sentiment_max` removed, `insight_type` added
- **Weekly Digest**: LLM digest prompt updated; digest content schema changes
- **Migration**: Backfill not possible for removed sentiment data; existing comments retain other LLM fields (category, topic, confidence)
