## Context

Noumena is YouTube-only today: TikTok connect endpoints are 501 stubs (internal/api/channel_placeholders.go) and the ingestion worker treats every non-YouTube job as a completed no-op (internal/ingestion/worker.go:216-219). The shared-channel model (one `platform_channels` row per real channel, per-user `user_channel_tracking`, comments ingested once) is platform-agnostic by construction — `platform` is a column, and access control, stars, digests, and export all join on the shared tables. TikTok therefore slots into the existing machinery rather than a new subsystem.

## Goals / Non-Goals

**Goals:**
- First-class TikTok connect: OAuth exchange storing encrypted per-user tokens, reusing `oauth_states`, token refresh, and primary-token promotion.
- Real TikTok comment ingestion through the Research API with the same job-claim/dedupe/backoff/dead-letter pipeline.
- 30-day backfill (Research API window), surfaced through the existing backfill-window UI.
- Frontend connect entry + platform badge; TikTok channels appear in feed/sentiment/export exactly like YouTube.

**Non-Goals:**
- Instagram/Meta — separate change.
- TikTok posting (replies/comments) — out of scope; ingestion is read-only.
- TikTok "Research API" upgrade path to "Content Posting API" — documented but not built.
- Legacy 501 placeholder removal outside the TikTok surface.

## Decisions

1. **Research API v2 as the only TikTok source.** Read-only comments + videos. Requires app-level review; ops runbook covers application.
2. **Platform switch in the worker, not new worker.** `runIngestion` dispatches on `platform`; a `tiktokFetcher` implements the same narrow fetcher interface as the YouTube client. Per-channel mutual exclusion remains claim-atomicity + `ON CONFLICT DO NOTHING`.
3. **TikTok backfill days restricted to {30} initially** — Research API only returns 30 days. `backfill_days` validation gains a per-platform allowed set.
4. **Token priority/promotion unchanged.** New tracker gets MAX+1; primary-token revocation promotes next tracker (existing logic is platform-agnostic).
5. **Quota model:** Research API daily cap (default 100 videos/day per app) is enforced via per-channel backoff + a shared daily budget counter in the scheduler; dead-letter + alert when exhausted, mirroring YouTube rate-limit handling.
6. **Config parity:** `TIKTOK_CLIENT_ID`, `TIKTOK_CLIENT_SECRET`, `TIKTOK_REDIRECT_URI` mandatory when `TIKTOK_ENABLED=true`; strict-config validated like YouTube keys.

## Risks / Trade-offs

- **Research API approval risk:** TikTok app review is manual and can be revoked (PRD §9). Mitigation: platform abstraction keeps YouTube unaffected; flag `TIKTOK_ENABLED` gates the whole surface (503 like `INSIGHTS_API_ENABLED`).
- **Fetch-depth limits:** Research API returns only video-level comment counts + samples in some endpoints; thread replies may be unavailable → document data-shape differences in the API reference; digest/insight pipelines are unaffected (they consume `comments`).
- **Backfill semantics:** 30-day max means a TikTok channel's history is shallower than YouTube's 90-180d; the backfill-window help page must state the per-platform bound.
- **New OAuth surface:** same CSRF/state patterns as YouTube; no new attack surface beyond scope already handled for Google/YouTube.