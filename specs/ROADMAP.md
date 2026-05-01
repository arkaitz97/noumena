# Noumena Implementation Roadmap

## Guiding Principles
1. **Backend-first, data-first**: Establish the database schema and core API before building frontend
2. **Auth is the foundation**: Everything depends on knowing who the user is
3. **Shared channel model is the differentiator**: Get the ingestion + LLM pipeline right early
4. **Vertical slices over horizontal layers**: Implement a feature end-to-end before moving to the next
5. **Defer "Could" and complex "Should" priorities**: Focus on Must-haves for v1.0

---

## Phase 1: Foundation (Weeks 1-2)
**Goal:** A working backend with user authentication, database, and basic API structure.

### Spec 01: Core Authentication & User Management
**Priority: CRITICAL | Dependencies: None**

This is the bedrock. Without auth, no other feature can function securely.

**What to implement:**
- Complete `users` table with all auth-related columns (token_version, failed_login_attempts, locked_until, deletion_requested_at)
- `email_verification_tokens`, `password_reset_tokens`, `refresh_tokens` tables
- All auth endpoints: `/auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout`, `/auth/logout-all`
- `/auth/verify-email`, `/auth/resend-verification`
- `/auth/forgot-password`, `/auth/reset-password`
- Google SSO: `/auth/google`, `/auth/google/callback`
- JWT token rotation (access 15min, refresh 30-day rolling)
- Brute-force protection (5 failures → 15min lock)
- Auth middleware with email verification gating
- Unverified account cleanup job (7-day deletion)

**Key decisions needed before starting:**
- Should Google SSO for auth use the same OAuth app/credentials as YouTube channel connection, or separate? (The PRD mentions both use Google OAuth but for different purposes. Using the same app with combined scopes is simpler for MVP, but separate apps is cleaner architecturally.)

**Files to modify:**
- `internal/db/schema.sql` — add tables/columns
- `internal/db/db.go` — add DBQuery methods
- `internal/auth/jwt.go` — implement refresh token logic
- `internal/api/auth_handlers.go` — full implementation
- `internal/api/middleware.go` — verification gate
- `internal/email/client.go` — verification + reset templates

---

## Phase 2: Channel Connection (Week 2-3)
**Goal:** Users can connect YouTube channels and the shared channel model works.

### Spec 02: Channel Management & YouTube OAuth
**Priority: CRITICAL | Dependencies: Spec 01**

**What to implement:**
- YouTube OAuth flow (separate from auth Google SSO if decided)
- Shared channel upsert logic (reuse `platform_channels` if exists)
- `user_channel_tracking` creation with encrypted tokens
- List, pause, resume, remove endpoints
- Primary token priority system (`token_priority`)
- Token expiry notifications (7 days before)
- Backfill window configuration (30/60/90/180 days)
- YouTube Data API v3 client for fetching channel info

**What to defer:**
- TikTok OAuth (CM-02: Should)
- Instagram OAuth (CM-03: Could)
- Advanced token promotion automation (handle basic case: manual or simple fallback)

**Files to modify:**
- `internal/db/schema.sql` — add backfill_days, token_expiry_notified_at
- `internal/db/channels.go` — upsert, list, pause, resume, remove
- `internal/db/tracking.go` — token management
- `internal/oauth/youtube.go` — OAuth exchange
- `internal/api/channel_handlers.go` — all endpoints
- `internal/email/client.go` — token expiry template

---

## Phase 3: Ingestion Pipeline (Week 3-4)
**Goal:** Comments are automatically fetched from YouTube and stored in the database.

### Spec 03: Comment Ingestion Pipeline
**Priority: CRITICAL | Dependencies: Spec 02**

**What to implement:**
- Background worker that polls every 30 minutes per channel
- Advisory lock (`pg_try_advisory_lock`) per channel
- YouTube Data API comment fetching (video list → comment threads)
- Deduplication via unique constraint on `(platform_channel_id, platform_comment_id)`
- Exponential backoff retry (1s, 2s, 4s, 8s, 16s → dead letter)
- Rate limit header parsing and backoff
- Backfill job for new channels (90-day default)
- Uses primary ingestion token (lowest token_priority)

**What to defer:**
- TikTok/Instagram ingestion (depends on deferred channel types)
- Sophisticated rate limit queuing (basic backoff is fine for MVP)

**Files to create/modify:**
- `internal/ingestion/worker.go` — core worker
- `internal/ingestion/scheduler.go` — dispatch logic
- `internal/ingestion/youtube_client.go` — API wrapper
- `internal/ingestion/lock.go` — advisory locks
- `internal/db/channels.go` — sync time, error tracking
- `internal/db/comments.go` — insert/upsert

---

## Phase 4: LLM Processing (Week 4-5)
**Goal:** Comments are classified, analyzed, and enriched with AI metadata.

### Spec 04: LLM Comment Processing
**Priority: CRITICAL | Dependencies: Spec 03**

**What to implement:**
- Batch processor (50 comments per batch)
- OpenAI GPT-4o integration for classification
- 8-category classification with confidence threshold (≥0.7)
- Sentiment scoring (-1.0 to +1.0)
- Topic extraction (≤10 words)
- Anthropic Claude fallback (basic failover after >5 errors in 60s)
- Toxic/spam flagging for insight exclusion
- Quota enforcement (check before processing, increment after)

**What to defer:**
- Custom prompt per channel (LLM-06: Could)
- Detailed cost tracking dashboard (basic increment is fine)
- Sophisticated quota attribution (attribute to primary token holder for MVP)

**Files to create/modify:**
- `internal/llm/client.go` — OpenAI + Anthropic clients
- `internal/llm/processor.go` — batch worker
- `internal/llm/prompts.go` — classification prompt
- `internal/db/comments.go` — update LLM data
- `internal/db/db.go` — increment comments used

---

## Phase 5: Insights API (Week 5-6)
**Goal:** Backend API serves filtered comment feeds, sentiment data, and trends.

### Spec 05: Insights Dashboard API
**Priority: HIGH | Dependencies: Spec 04**

**What to implement:**
- `/api/comments/feed` with full filtering (category, channel, date, sentiment, search)
- `/api/comments/saved` for starred comments
- `POST/DELETE /api/comments/{id}/star`
- `/api/insights/sentiment` (7d/30d/90d aggregates)
- `/api/insights/toxicity-shield`
- `/api/insights/trends` (top topics, categories)
- `/api/comments/export` with sync (<50K) and async (>50K) paths
- Composite indexes for performance

**What to defer:**
- Materialized views (can add if p95 > 2s in testing)
- Object storage for exports (write to local disk with 24h cleanup for MVP)

**Files to create/modify:**
- `internal/api/comment_handlers.go` — extend significantly
- `internal/db/comments.go` — feed queries, aggregations
- `internal/export/worker.go` — export generation

---

## Phase 6: Weekly Digest (Week 6)
**Goal:** Automated weekly insight reports are generated and stored.

### Spec 06: Weekly Digest Generation
**Priority: HIGH | Dependencies: Spec 04, Spec 05**

**What to implement:**
- Monday 08:00 UTC cron trigger
- Per-user digest generation job
- Skip if < 10 comments
- LLM summarization prompt for top 5 ideas + top 5 questions
- Sentiment trend comparison (current vs previous week)
- Engagement health score calculation
- Retry 3 times over 2 hours, mark deferred on failure
- Store in `weekly_digests` table

**What to defer:**
- Email delivery of digest (implement in Phase 8 with email system fully wired)

**Files to create/modify:**
- `internal/llm/digest_worker.go` — generation logic
- `internal/db/digests.go` — queries

---

## Phase 7: Billing & Stripe (Week 7)
**Goal:** Users can upgrade plans, and usage limits are enforced.

### Spec 07: Billing & Subscriptions
**Priority: HIGH | Dependencies: Spec 01**

**What to implement:**
- Stripe Checkout integration for all plans + annual discount
- Customer Portal for subscription management
- Webhook handler (`/webhooks/stripe`) with idempotency
- Plan enforcement: channel limits, comment quota
- Quota reset on billing anniversary
- Overages: $0.50 per 1K comments (basic tracking)
- Grace period on payment failure (3 days)

**What to defer:**
- Complex downgrade channel selection logic (simpler: pause excess channels alphabetically, let user reactivate)
- Detailed usage history charts

**Files to create/modify:**
- `internal/billing/client.go` — Stripe wrapper
- `internal/api/billing_handlers.go` — checkout + webhooks
- `internal/db/db.go` — subscription updates

---

## Phase 8: Data Lifecycle & Email (Week 7-8)
**Goal:** GDPR compliance, account deletion, and transactional emails work.

### Spec 08: Data Lifecycle, Account Deletion & GDPR
**Priority: MEDIUM | Dependencies: Spec 01, Spec 02, Spec 07**

### Plus: Email system completion
**Dependencies: All previous specs**

**What to implement:**
- Account deletion flow (2-step confirmation, 48h reversible window)
- GDPR data export (ZIP of JSON, 24h SLA)
- Shared comment purge logic (only when last tracker)
- OAuth token revocation on deletion
- Primary token promotion before revocation
- Chunked deletion for large channels (10K rows/transaction)
- All transactional emails:
  - Verification email
  - Password reset
  - Login lockout alert
  - Token expiry warning
  - Payment failure + grace period
  - Trial expiry reminder
  - Digest email (weekly)
  - Export ready notification
  - Account deletion confirmation

**Files to create/modify:**
- `internal/lifecycle/deletion_worker.go` — new
- `internal/lifecycle/export_worker.go` — new
- `internal/email/client.go` — all templates
- `internal/api/auth_handlers.go` — account endpoints

---

## Phase 9: Frontend - Auth & Onboarding (Week 8-9)
**Goal:** Users can register, login, verify email, and connect their first channel.

### Spec 09: Frontend - Authentication & Onboarding
**Priority: HIGH | Dependencies: Spec 01, Spec 02**

**What to implement:**
- All auth views (login, signup, verify, forgot/reset password)
- Pinia auth store with token management
- Axios interceptors for refresh + redirect
- Router guards (auth, verification)
- Google SSO popup flow
- Onboarding wizard (connect YouTube, backfill progress)
- Dashboard layout shell

**Files to create/modify:**
- `frontend/src/router/index.ts`
- `frontend/src/stores/auth.ts`
- `frontend/src/services/api.ts`
- `frontend/src/views/auth/*.vue`
- `frontend/src/views/dashboard/DashboardLayout.vue`

---

## Phase 10: Frontend - Dashboard & Settings (Week 9-10)
**Goal:** Full-featured dashboard for insights, saved comments, and settings.

### Spec 10: Frontend - Dashboard & Settings
**Priority: HIGH | Dependencies: Spec 05, Spec 09**

**What to implement:**
- Insights feed with infinite scroll and filters
- Comment cards with star toggle
- Sentiment chart and toxicity shield
- Weekly digest card
- Saved comments view
- Channel management (pause/resume/remove, connect new)
- Billing page (usage, upgrade, Stripe portal)
- Account settings (profile, export, deletion)

**Files to create/modify:**
- `frontend/src/views/dashboard/InsightsFeed.vue`
- `frontend/src/views/dashboard/SavedCommentsView.vue`
- `frontend/src/views/settings/*.vue`
- `frontend/src/components/*.vue`
- `frontend/src/stores/*.ts`

---

## Summary Order

| Order | Spec | Phase | Duration | Blockers |
|-------|------|-------|----------|----------|
| 1 | 01 - Core Auth | Foundation | 1 week | None |
| 2 | 02 - Channels | Foundation | 1 week | Spec 01 |
| 3 | 03 - Ingestion | Foundation | 1 week | Spec 02 |
| 4 | 04 - LLM Processing | Core Intelligence | 1 week | Spec 03 |
| 5 | 05 - Insights API | Core Intelligence | 1 week | Spec 04 |
| 6 | 06 - Weekly Digest | Core Intelligence | 0.5 week | Spec 04, 05 |
| 7 | 07 - Billing | Monetization | 1 week | Spec 01 |
| 8 | 08 - Data Lifecycle | Compliance | 1 week | Spec 01, 02, 07 |
| 9 | 09 - Frontend Auth | UI | 1 week | Spec 01, 02 |
| 10 | 10 - Frontend Dashboard | UI | 1-2 weeks | Spec 05, 09 |

**Total estimated timeline: 10-11 weeks for v1.0 MVP**

---

## Risk Mitigation & Parallelization

### Can be parallelized
- **Frontend (Specs 09-10)** can start once Spec 01 (Auth API) and Spec 02 (Channels API) are stable, even while Ingestion/LLM are being built. Frontend can use mock data for insights during development.
- **Billing (Spec 07)** can be developed in parallel with Ingestion/LLM since it's mostly independent Stripe integration.

### Must be sequential
- Ingestion → LLM → Insights API (data flows through these)
- Auth → Everything else (all endpoints require auth)

### Early validation points
1. After Spec 01: Run end-to-end auth flow (register → verify → login → refresh → logout)
2. After Spec 03: Connect a real YouTube channel, verify comments appear in DB
3. After Spec 04: Verify LLM classifications are reasonable on real comments
4. After Spec 05: Dashboard API returns filtered feed within 2s

---

## Open Questions for Engineering Kickoff

Before starting implementation, the following questions from the PRD should be resolved:

1. **Google OAuth scope sharing**: Should the Google OAuth app used for user authentication (Google SSO) be the same as the one used for YouTube channel connection? Same app with combined scopes is simpler; separate apps is more secure but requires managing two sets of credentials.

2. **LLM batching scope**: Should batches be per-channel only, or can we batch across channels? Per-channel is safer for data isolation (no cross-user leakage in prompts) but slightly less cost-efficient. **Recommendation: per-channel for v1.**

3. **Job queue choice**: The PRD suggests River (Go-native, PostgreSQL-backed). The current code uses background goroutines. Should we adopt River now or stick with simple polling for MVP? **Recommendation: Simple polling with `time.Ticker` for v1, migrate to River if scale demands it.**

4. **Quota attribution**: Since comments are shared and processed once, which user gets charged? Primary token holder (the one whose OAuth token fetches the comments) is simplest. Is this acceptable?

5. **Export storage**: For GDPR exports and large CSVs, should we implement S3/R2 integration or use local disk with cleanup? **Recommendation: Local disk with 24h cleanup for MVP (under 10MB typical).**

6. **Refresh token storage**: Should we store full refresh tokens in PostgreSQL or use a separate Redis/cache layer? **Recommendation: PostgreSQL is fine for v1 (the PRD already has a refresh_tokens table design).**
