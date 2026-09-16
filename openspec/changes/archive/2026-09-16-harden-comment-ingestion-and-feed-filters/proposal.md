# Proposal: Harden YouTube Comment Ingestion and Insight Feed Filters

## Why

The YouTube API client paginates individual responses correctly, but the surrounding ingestion workflow can still lose data. Channel polling has no durable channel-wide cursor, backfills can rediscover the newest videos after hitting a ceiling, old threads are skipped before late replies are inspected, and comment-page errors can be swallowed while the job is advanced. The current backfill horizon is also based on video publish time rather than comment publish time. These failures create permanent gaps while making the job appear healthy.

The Insight Feed has a separate set of correctness problems. Insight-type filtering only examines the first JSON insight even though a comment may contain several, the Ideas preset currently combines predicates with `AND` instead of its documented `OR`, date-only end dates exclude the selected day, an empty category selection is indistinguishable from the default category filter, and URL/store/widget state can become stale or leak between feed views.

## What Changes

- Add durable, transactional YouTube discovery, comment-page, and reply reconciliation checkpoints for poll and backfill jobs.
- Make partial, deadline, quota, and API-error stops resumable; do not advance a synchronization watermark past unscanned data.
- Enumerate backfill videos independently from comment-date filtering so recent comments on older videos are eligible.
- Preserve actual reply parent IDs and define the retrievable moderation/scope boundary explicitly.
- Make feed insight predicates inspect every actionable insight and add a first-class intent/filter contract for `Ideas`, `Actionable`, and advanced filters.
- Define inclusive date-only ranges, explicit empty category filters, canonical defaults, escaped search, and independent route state.
- Propagate the same filter scope to dashboard widgets and make Saved comments reliably discoverable.

## Capabilities

### New Capabilities

- `reliable-youtube-comment-ingestion`: Resumable channel polling/backfill, complete reply walks, error handling, and explicit retrieval scope.

### Modified Capabilities

- `insights-api`: Correct JSON insight matching, filter/date/search semantics, intent predicates, and scoped aggregates.
- `frontend-dashboard`: Canonical filter state, intent tabs, widget synchronization, and saved-feed behavior.

## Impact

- `internal/ingestion/` — persist and resume channel/video/reply progress; return comment API errors; correct reply parents and channel scope.
- `internal/db/` — add migration-backed progress state and query methods; update feed predicates and filter parsing.
- `internal/api/` — expose the clarified feed/intent contract and pass scope consistently to aggregates.
- `frontend/src/` — synchronize route, store, controls, widgets, and Saved view with the backend contract.
- Tests and operational telemetry — add interruption, omission, filter truth-table, and multi-scope coverage.

## Non-goals

- Recovering comments that YouTube no longer makes available to the authenticated retrieval scope.
- Posting replies or changing YouTube moderation decisions.
- Replacing the existing deduplication model or redesigning the dashboard visuals.

## Success Criteria

1. A poll or backfill interrupted at any page resumes from durable progress and eventually reaches completion without relying on the first page being new.
2. A retrievable comment or reply is not silently lost because of a page ceiling, old parent thread, swallowed API error, or video age.
3. API and UI filters return the same rows for equivalent requests, including second-or-later insights, end dates, empty categories, intent tabs, and wildcard characters.
4. Dashboard aggregates describe the same channel/date/filter scope as the visible feed.
