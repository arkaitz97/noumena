## 1. OpenSpec and contracts

- [x] 1.1 Document the effective YouTube retrieval scope (published vs creator-authorized moderation states, video-associated vs channel-level comments) and telemetry fields.
- [x] 1.2 Define the shared `FeedScope`, canonical default categories, intent predicates, date-only rules, explicit-empty encoding, and literal-search behavior in the API/frontend contract.

## 2. Durable YouTube progress

- [x] 2.1 Add a versioned SQLite migration for channel-level poll/backfill discovery checkpoints and any required per-video reply checkpoint fields.
- [x] 2.2 Add DB models and transactional methods to read, advance, pause, retry, and complete discovery/comment/reply checkpoints.
- [x] 2.3 Persist checkpoint plus comment batch atomically; retain progress on deadline, request/video/page ceiling, quota stop, cancellation, and process restart.
- [x] 2.4 Make scheduler decisions use durable backfill completion/progress rather than treating `last_synced_at` as proof that an incomplete scan is done.

## 3. YouTube worker and client correctness

- [x] 3.1 Resume channel poll walks from their durable page/boundary checkpoint and apply a bounded overlap/reconciliation pass before advancing the watermark.
- [x] 3.2 Resume backfill upload discovery after `MaxVideosPerJob` and continue until discovery is complete; do not repeatedly start at the newest upload.
- [x] 3.3 Remove video-publish-date filtering as the backfill completeness gate; filter candidate comments by comment `PostedAt` instead.
- [x] 3.4 Reconcile old threads whose reply counts/check timestamps require it and paginate every available reply page.
- [x] 3.5 Return `commentThreads.list`/`comments.list` errors to the worker; classify only valid comments-disabled responses as skips and preserve checkpoints for retry.
- [x] 3.6 Preserve the reply resource's actual parent ID, reject or separately identify channel-level rows without a video association, and select/report the intended auth/moderation scope.
- [x] 3.7 Add worker/client tests for partial channel polls, resumed backfills, old-video recent comments, late replies, page errors, invalid tokens, parent IDs, channel-only rows, and auth scope.

## 4. Backend Insight Feed contract

- [x] 4.1 Replace first-element JSON extraction with an `EXISTS`/`json_each` predicate for insight-type filtering and add regression coverage for a matching second insight.
- [x] 4.2 Add typed intent handling so Ideas uses category-or-insight semantics and Actionable has one documented source of truth.
- [x] 4.3 Distinguish omitted categories from explicit empty categories, centralize default categories, and ensure direct API and frontend-generated requests agree.
- [x] 4.4 Normalize date-only `date_to` to an exclusive next-day boundary, preserving exact timestamp behavior for datetime inputs.
- [x] 4.5 Escape literal search input for SQLite `LIKE` and add `%`, `_`, and escape-character tests.
- [x] 4.6 Apply the full feed scope to insight aggregates and define multi-channel and custom/all-time range behavior.
- [x] 4.7 Add API/DB tests for combined filters, intent truth tables, boundaries, aggregate scope, and Saved comments without hidden default filters.

## 5. Frontend state and filter behavior

- [x] 5.1 Introduce a canonical `FeedScope` serializer/parser that replaces state on route hydration and encodes explicit empty categories.
- [x] 5.2 Isolate Insights Feed and Saved view filter state, and reset stale intent/range/view state when switching presets or routes.
- [x] 5.3 Update intent tabs to use the API intent contract, especially Ideas `OR` behavior and Actionable semantics.
- [x] 5.4 Make date-range controls update both range and dates; include the selected end day in the UI/API request.
- [x] 5.5 Send the canonical default category set and preserve an intentional empty selection.
- [x] 5.6 Pass the complete selected scope to widgets, including all selected channels and custom/all-time dates; add loading/empty-state behavior for scope changes.
- [x] 5.7 Ensure Saved defaults show every starred comment and add component/store tests for route isolation and scope serialization.

## 6. Verification and rollout

- [x] 6.1 Run Go ingestion, DB, and API tests plus frontend unit tests and build.
- [x] 6.2 Add end-to-end coverage for interrupted/resumed ingestion fixtures and feed filter/UI scenarios.
- [x] 6.3 Add metrics/logging for checkpoint age, resumed pages, incomplete ceiling stops, reply reconciliation, skipped scope rows, and filter-scope mismatches.
- [x] 6.4 Roll out behind existing ingestion/feed flags if needed, verify migration on a copy of production data, and document recovery/reconciliation operations.
