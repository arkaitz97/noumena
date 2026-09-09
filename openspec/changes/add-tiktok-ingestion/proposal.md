# Proposal: Add TikTok Ingestion

## Why

TikTok is the second-largest creator platform and PRD requirement CM-02 (Should). Today the product is YouTube-only: `/api/channels/connect/tiktok` and `/api/channels/exchange/tiktok` return 501 (internal/api/channel_placeholders.go:7-14) and ingestion marks any non-YouTube job "completed" without calling any API (internal/ingestion/worker.go:216-219). Multi-platform support is core to the positioning ("creators on YouTube, TikTok, Instagram…") and to the Agency persona, who manages creators across platforms.

## What Changes

- Real TikTok OAuth 2.0 connect flow (Research API): `GET /api/channels/connect/tiktok`, `POST /api/channels/exchange/tiktok` — replaces the 501 stubs.
- TikTok ingestion in the worker: fetch videos + comments via TikTok Research API, same shared-channel model (one `platform_channels` row per real channel, comments ingested once, shared across trackers).
- Backfill support for TikTok channels (Research API window is 30 days; backfill window options reduced accordingly).
- Token lifecycle: encrypted per-user tokens, primary-token promotion, token-expiry notification — reusing the existing `user_channel_tracking` machinery.
- Frontend: TikTok entry in channel connect flow + ChannelCard platform badge.
- Rate-limit handling for the Research API (daily quota) with per-channel backoff and dead-lettering, matching YouTube behavior.

## Capabilities

### New Capabilities
- `tiktok-ingestion`: platform-specific TikTok OAuth exchange, comment fetching via the Research API, and quota handling.

### Modified Capabilities
- `channel-management`: add TikTok as a connectable platform (OAuth flow, token storage, pause/resume/remove, backfill-window semantics).
- `ingestion`: add TikTok comment fetching to the worker's platform switch; non-YouTube no-op path is replaced by real fetching for TikTok.
- `frontend-dashboard`: TikTok connect entry and platform badges in the channel UI.

## Impact

- `internal/oauth/` — new `tiktok.go` (OAuth config, exchange, revoke).
- `internal/ingestion/` — youtube.go/worker.go platform abstraction; Research API client; scheduler respects TikTok daily quota.
- `internal/api/channel_handlers.go` + `channel_placeholders.go` — real handlers replace stubs; `PlatformValidationMiddleware` mounted.
- `internal/db/schema.sql` — no structural change expected (platform is already a column); check `platform_channels.platform` constraints.
- Config: `TIKTOK_CLIENT_ID`, `TIKTOK_CLIENT_SECRET`, `TIKTOK_RESEARCH_*` env vars.
- Ops runbook: `docs/ops/runbooks/tiktok-oauth-setup.md`.