# Design: Harden YouTube Comment Ingestion and Insight Feed Filters

## Context

The current client correctly follows `nextPageToken` within one API walk, but progress is held in local iterator state. The worker then uses `last_synced_at` and per-video watermarks as if they represented a complete channel scan. A bounded run can therefore stop after seeing only the newest page while later treating older pages as permanently covered. The same distinction is needed for backfills: queue-job completion, page progress, and full backfill completion are different states.

The feed stores actionable insights as a JSON array in SQLite. Some queries use `json_each`, while the main feed predicate extracts only `$[0].type`. Frontend presets are currently translated into ordinary filter fields, so the backend cannot express the documented `OR` semantics for Ideas. Filter absence, explicit emptiness, date-only values, and route changes are also currently conflated.

## Goals

- Make ingestion progress durable at every resumable boundary and safe to replay.
- Distinguish a bounded/partial run from a fully synchronized channel or backfill.
- Define exactly which YouTube comments Noumena promises to retrieve.
- Make feed, intent, aggregate, URL, and Saved-view semantics deterministic and shared.

## Non-goals

- Guaranteeing access to deleted, private, disabled, or moderation-held comments when the selected YouTube credentials cannot retrieve them.
- Adding a second ingestion worker or changing the SQLite job-claim mechanism.
- Introducing client-side filtering that differs from the API result set.

## Decisions

### 1. Durable progress is separate from watermarks

Add migration-backed channel-level progress for discovery and poll scans, and extend the existing per-video state for comment and reply walks. The state must be able to represent:

- sync phase (`poll`, `backfill`, or reply reconciliation);
- uploads/video-discovery page token and current video;
- comment-thread page token and the scan boundary/high-watermark;
- reply page token, remote reply count, and last completed reply scan;
- `in_progress`, `complete`, `paused`, and `error` details with timestamps.

A comment batch and its next checkpoint are committed atomically. Replaying a committed page remains safe through the existing `platform_comment_id` uniqueness/upsert behavior. A token that expires or becomes invalid causes a bounded overlap/restart from a known safe boundary; it must not be treated as a completed walk.

Queue jobs may finish with a normal ceiling status so the scheduler can defer work, but the durable progress record must remain incomplete and the scheduler must enqueue a continuation. `last_synced_at` and backfill completion may advance only after the relevant scan reaches its declared boundary.

### 2. Polls use a resumable channel scan plus overlap

A poll persists its channel page token and scan boundary after each successful page. It does not stop permanently just because the first older thread or video was encountered. If a deadline, request ceiling, quota stop, or transient error interrupts the walk, the next poll resumes the checkpoint and retains the prior watermark.

Completed polls keep a small overlap/reconciliation window. This permits late replies and edits to old top-level threads to be found even when the top-level comment predates the watermark. A thread is eligible for reply reconciliation when it is in the overlap or its stored remote reply count/last reply check indicates that it needs a refresh.

### 3. Backfills resume video discovery and filter by comment date

Backfill video discovery has its own durable cursor. Hitting `MaxVideosPerJob` advances the cursor only through videos actually examined; the next job continues with the next upload rather than rediscovering the newest video.

Backfill discovery must not exclude a video solely because its publish date is older than the requested comment window. It enumerates uploads until discovery is complete, then applies the requested `PostedAt` horizon to comments. This preserves recent comments on evergreen or old videos. A backfill is complete only when video discovery and all eligible comment/reply walks are complete.

### 4. Errors stop progress safely

Any non-success from `commentThreads.list` or `comments.list`, other than an explicitly classified comments-disabled response, is returned to the worker. The worker records the error, preserves the last committed checkpoint, and schedules retry/backoff. It must not clear a checkpoint, advance a watermark, or mark the scan fully complete after a swallowed page error.

### 5. Retrieval scope and reply hierarchy are explicit

The default product scope is all published, video-associated comments retrievable by the configured credential. Channel-wide API results that have no video association are excluded from the video comment corpus (or stored in a separately identified channel-comment scope); they must not silently appear as video comments with a fabricated video ID.

For connected creator channels, ingestion uses the owner-authorized token when creator-visible moderation states are part of the selected scope. API-key/public ingestion is documented as published-only. The API response and telemetry expose the effective scope so “all comments” is not confused with “all comments visible to an API key.”

When a reply is normalized, `parent_comment_id` comes from the reply resource's actual parent ID. The inline thread parent is only a fallback when the resource omits it. Reply fetching follows every available `comments.list` page and records remote-vs-local counts; it does not claim unsupported YouTube reply depths that the API cannot return.

### 6. Feed filtering uses a typed contract

The feed query parser distinguishes an omitted parameter from an explicitly empty one. The canonical semantics are:

- omitted `categories`: default high-signal categories;
- explicit empty `categories=`: match no categories;
- selected categories: match any selected category;
- `insight_type`: match if any element in `actionable_insights` has that type, using `json_each`/`EXISTS`;
- `intent=ideas`: category `content_idea` OR insight type `video_idea`, with other advanced filters applied as `AND` outside that preset;
- `intent=actionable`: comments with at least one actionable insight, with other advanced filters applied as `AND`;
- `intent=all`: no intent predicate;
- date-only `date_from`: start of that UTC calendar day;
- date-only `date_to`: exclusive start of the following UTC calendar day, so the selected day is included;
- search: literal substring matching with SQL `LIKE` metacharacters escaped. Wildcard behavior is not implicit.

The default high-signal category list is defined once in the API contract and mirrored by the frontend: `question`, `content_idea`, `collaboration_ask`, `praise`, and `constructive_criticism`. `other` remains selectable explicitly but is not accidentally added by one client only.

### 7. URL, store, widgets, and Saved view share one scope model

The frontend has one canonical serializable `FeedScope` model. Route hydration replaces the complete scope, including defaults, instead of applying only keys present in a new URL. Feed and Saved routes have independent scope instances or an explicit reset boundary, so filters do not leak between views. Selecting/resetting an intent updates the active tab and clears stale intent state.

All summary widgets receive the same selected channel set, date range, search, category, insight, and intent scope as the feed. Multi-channel selections aggregate across all selected channels. Relative range shortcuts update the canonical date/range state together; custom/all-time selections cannot leave widgets using the previous range.

Saved comments default to all comments the user starred, sorted by `starred_at`, without silently applying the normal high-signal/toxicity feed defaults. If the UI offers additional Saved filters, they are explicit and visible.

## Data and API Shape

The implementation should add a versioned SQLite migration for the channel/discovery checkpoint and any missing reply checkpoint fields. New DB methods should expose checkpoint read/write/complete operations rather than allowing handlers to update progress with ad hoc SQL. The feed API should accept the optional `intent` field and preserve backward compatibility for existing category/insight/date/search parameters.

The exact table name may follow the repository's existing `video_sync_state` naming, but the invariant is required: one durable source of truth for channel discovery progress and one for each resumable video/reply walk.

## Verification Strategy

- Use in-memory SQLite migration tests for checkpoint persistence, atomic batch/checkpoint commits, explicit empty filters, JSON arrays with the match in position two, date boundaries, and escaped search.
- Use fake YouTube clients to interrupt after each page, ceiling, token invalidation, reply page, and error; assert the next run resumes and no item is permanently skipped.
- Add frontend unit tests for URL replacement, route isolation, intent truth tables, range synchronization, default categories, widget scope, and Saved defaults.
- Add API/integration tests covering equivalent frontend-generated and direct query parameters.
