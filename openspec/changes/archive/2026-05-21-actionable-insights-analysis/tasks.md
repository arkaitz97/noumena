## 1. Database Migration

- [x] 1.1 Create migration: add `actionable_insights JSONB` column to `comments` table with default `[]`
- [x] 1.2 Create migration: add GIN index on `actionable_insights` column
- [x] 1.3 Create migration: drop `sentiment_score` column from `comments` table
- [x] 1.4 Create migration: drop sentiment-related indexes
- [x] 1.5 Create migration: create `mv_actionable_insights_agg` materialized view for insight aggregation
- [x] 1.6 Create migration: add refresh function for materialized view

## 2. LLM Processing — Prompt & Parser

- [x] 2.1 Update `SystemPrompt` in `internal/llm/prompts.go` to remove sentiment and add actionable_insights extraction instructions
- [x] 2.2 Update `LLMResult` struct: remove `Sentiment` field, add `ActionableInsights []Insight` field
- [x] 2.3 Add `Insight` struct with fields: `Type`, `Description`, `Confidence`
- [x] 2.4 Update `ParseClassificationResponse` to validate insight types against allowed set
- [x] 2.5 Update `ParseClassificationResponse` to enforce max 5 insights per comment
- [x] 2.6 Update `ParseClassificationResponse` to enforce 200-char description limit with truncation
- [x] 2.7 Update `ParseClassificationResponse` to apply confidence threshold to individual insights
- [x] 2.8 Remove sentiment range validation from parser
- [x] 2.9 Update all test files in `internal/llm/` to use new response format (remove sentiment, add insights)

## 3. LLM Processing — Persistence

- [x] 3.1 Update `UpdateCommentLLMData` in `internal/db/comments.go` to accept `actionableInsights JSONB` instead of `sentiment float64`
- [x] 3.2 Update `internal/db/db.go` interface signature for `UpdateCommentLLMData`
- [x] 3.3 Update `internal/llm/processor.go` to pass actionable_insights to persistence call
- [x] 3.4 Update `internal/llm/processor_test.go` fake query to match new signature
- [x] 3.5 Update `internal/api/api_test.go` MockQuery to match new signature

## 4. Database Query Layer

- [x] 4.1 Update `Comment` model in `internal/db/models.go`: remove `SentimentScore`, add `ActionableInsights` field
- [x] 4.2 Update all SELECT queries in `internal/db/comments.go` to select `actionable_insights` instead of `sentiment_score`
- [x] 4.3 Update `internal/db/comments_feed.go`: remove sentiment sort options, add `InsightType` filter param
- [x] 4.4 Update `BuildFeedQuery` to handle `insight_type` filter (JSONB containment query)
- [x] 4.5 Remove `SentimentMin`/`SentimentMax` from `FeedFilters` struct
- [x] 4.6 Remove sentiment sort SQL mappings (`sentiment_desc`, `sentiment_asc`)
- [x] 4.7 Update `internal/db/comments_search.go` queries to select `actionable_insights` instead of `sentiment_score`
- [x] 4.8 Update `internal/db/comments_feed_db.go`: replace `GetSentimentAggregates` with `GetTopInsights` and `GetInsightDistribution`
- [x] 4.9 Remove `SentimentBucket` struct and sentiment aggregate SQL queries
- [x] 4.10 Update `internal/db/lifecycle.go` queries that reference `sentiment_score`
- [x] 4.11 Update `internal/db/digests.go` queries that reference `sentiment_score`
- [x] 4.12 Update `internal/db/comments_feed_test.go` to test insight_type filter and remove sentiment tests

- [x] 5.8 Update `LatestDigest` handler response to include `top_actionable_insights` instead of `sentiment_trend`

## 6. Weekly Digest

- [x] 6.1 Update `internal/llm/digest_llm.go` digest prompt: replace sentiment_trend with top_actionable_insights section
- [x] 6.2 Update `internal/llm/digest_aggregates.go`: remove `computeSentimentAverage`, `computeSentimentQuality`, `SentimentTrend` struct
- [x] 6.3 Add `computeTopActionableInsights` function to aggregate insights for digest
- [x] 6.4 Update `DigestTruth` struct: replace `SentimentTrend` with `TopActionableInsights`
- [x] 6.5 Update `BuildDigestPrompt` to pass actionable insights data instead of sentiment data
- [x] 6.6 Update `internal/llm/digest_client.go` response struct
- [x] 6.7 Update `internal/llm/digest_worker_test.go` test fixtures
- [x] 6.8 Update `internal/llm/digest_llm_test.go` test fixtures

## 7. Export

- [x] 7.1 Update `internal/export/encoder.go`: replace `sentiment_score` column header with `actionable_insights`
- [x] 7.2 Update CSV row encoding to serialize actionable_insights JSON array as string
- [x] 7.3 Update `exportRow` struct in encoder.go
- [x] 7.4 Update `internal/export/worker.go`: replace `SentimentMin`/`SentimentMax` with `InsightType` in export filters
- [x] 7.5 Update `internal/export/encoder_test.go` to test new column format

## 8. Frontend — Remove Sentiment Components

- [x] 8.1 Remove sentiment trend chart component from insights dashboard
- [x] 8.2 Remove sentiment filter controls from comment feed UI
- [x] 8.3 Remove sentiment score display from comment cards in feed
- [x] 8.4 Remove `sentiment_desc`/`sentiment_asc` sort options from feed sort dropdown
- [x] 8.5 Update feed API call to remove `sentiment_min`/`sentiment_max` params

## 9. Frontend — Add Actionable Insights Components

- [x] 9.1 Create `ActionableInsightsPanel` component for displaying insights grouped by type
- [x] 9.2 Create `InsightTypeDistributionChart` component (bar or pie chart)
- [x] 9.3 Create `InsightCard` component for displaying individual insights on comment cards
- [x] 9.4 Add `insight_type` filter dropdown to comment feed UI
- [x] 9.5 Add actionable insights panel to insights dashboard view
- [x] 9.6 Add insight type distribution chart to insights dashboard view
- [x] 9.7 Update feed API integration to use `insight_type` param
- [x] 9.8 Update digest display to show `top_actionable_insights` instead of `sentiment_trend`

## 10. Testing & Verification

- [x] 10.1 Run all backend tests: `go test ./...`
- [x] 10.2 Run all frontend tests
- [x] 10.3 Verify migration runs cleanly on fresh database
- [x] 10.4 Verify migration runs cleanly on database with existing data
- [x] 10.5 Verify LLM processor correctly extracts and persists actionable_insights
- [x] 10.6 Verify new API endpoints return correct data
- [x] 10.7 Verify feed filtering by insight_type works correctly
- [x] 10.8 Verify weekly digest generates with actionable insights content
- [x] 10.9 Verify export includes actionable_insights column
- [x] 10.10 Verify frontend renders actionable insights correctly

## 11. Documentation & Cleanup

- [x] 11.1 Update `.env.example` if any new env vars are added
- [x] 11.2 Update API documentation for changed endpoints
- [x] 11.3 Remove any dead code references to sentiment_score
- [x] 11.4 Archive the OpenSpec change after implementation
