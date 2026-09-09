## ADDED Requirements

### Requirement: TikTok comment fetching

The ingestion worker SHALL fetch videos and comments for `platform = "tiktok"` channels through the TikTok Research API, reusing the job-claim, deduplication (`ON CONFLICT DO NOTHING` on `platform_comment_id`), retry/backoff, and dead-letter pipeline used for YouTube. The non-YouTube "mark completed without API calls" no-op SHALL no longer apply to TikTok.

#### Scenario: Ingestion job for a TikTok channel
- **WHEN** the scheduler enqueues a poll or backfill job for a TikTok channel
- **THEN** the worker fetches from the Research API and inserts comments deduplicated by `platform_comment_id`

#### Scenario: Research API rate limit
- **WHEN** the Research API returns a rate-limit response
- **THEN** the worker backs off per the existing retry budget and dead-letters after exhaustion, alerting ops

### Requirement: TikTok ingestion feature gating

The worker SHALL skip TikTok channels without error when the TikTok flag is disabled, and SHALL honor the daily Research API budget shared across channels.

#### Scenario: TikTok disabled in worker
- **WHEN** `TIKTOK_ENABLED=false` and a TikTok job is claimed
- **THEN** the job is marked completed without API calls and no error is raised

#### Scenario: Daily budget exhausted
- **WHEN** the shared daily Research API budget is exhausted
- **THEN** the worker parks TikTok ingestion until the budget resets and logs the pause