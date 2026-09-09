## 1. TikTok OAuth

- [ ] 1.1 Add `internal/oauth/tiktok.go`: OAuth config, BuildAuthURL (offline + consent), ExchangeCode (reject missing refresh token), ExchangeAndFetchChannel, RevokeToken — mirroring youtube.go
- [ ] 1.2 Add `TIKTOK_CLIENT_ID`, `TIKTOK_CLIENT_SECRET`, `TIKTOK_REDIRECT_URI`, `TIKTOK_ENABLED` to `internal/config/config.go` + `.env.example` (parity test enforces)
- [ ] 1.3 Replace 501 stubs: implement `ConnectTikTok` + `ExchangeTikTok` in channel handlers (state persist/consume, plan-limit 402, token encrypt, shared upsert, backfill enqueue)
- [ ] 1.4 Mount TikTok routes + `TIKTOK_ENABLED` 503 gate; keep `PlatformValidationMiddleware` for validation
- [ ] 1.5 Unit tests: BuildAuthURL scopes/state, exchange success/reuse/invalid-state/no-refresh-token/409, 503 when disabled

## 2. Ingestion

- [ ] 2.1 Add Research API client `internal/ingestion/tiktok.go`: videos + comments fetch, dedupe keys, rate-limit header parsing
- [ ] 2.2 Extend worker platform switch: TikTok poll + backfill paths; remove TikTok from the non-YouTube no-op branch
- [ ] 2.3 Per-platform backfill windows: allowed set {youtube: 30/60/90/180, tiktok: 30}; clamp + reflect in status response
- [ ] 2.4 Shared daily Research API budget counter in scheduler; park + log when exhausted
- [ ] 2.5 Worker tests: fetch happy path, dedupe, rate-limit backoff, disabled-flag skip, budget-exhausted park

## 3. Frontend

- [ ] 3.1 Channel connect flow: TikTok option (when flag exposed via status endpoint) + platform badge on ChannelCard
- [ ] 3.2 Backfill-window UI shows 30-day bound for TikTok channels
- [ ] 3.3 Component tests for badge + connect entry

## 4. Docs & ops

- [ ] 4.1 Runbook `docs/ops/runbooks/tiktok-oauth-setup.md` (Research API application, scopes, review)
- [ ] 4.2 Update `docs/api/channels.md` + `docs/help/backfill-window.md` (per-platform window bound)
- [ ] 4.3 Update DEFERRED_TASKS note: placeholder resolution + verify `channel_placeholders_test.go` covers new handlers