# Product Requirements Document (PRD)
## Noumena — AI-Powered Growth Intelligence for Content Creators

> **Version:** 1.3  
> **Date:** 2026-03-13  
> **Status:** Draft — Ready for Engineering Review  
> **Author:** Product-Architect Skill (Senior Product Architect AI)  
> **Primary Downstream Consumer:** API Designers & Backend Implementers  

---

## 1. Executive Summary

### The Problem
Content creators on YouTube, TikTok, Instagram, and other platforms are drowning in comment noise. For every 100 comments received, fewer than 10 contain actionable growth signals (feature requests, content gaps, audience sentiment, collaboration opportunities). The rest is spam, toxicity, and generic reactions. Manually reading tens of thousands of comments per week is not scalable — it burns creator time and exposes them to psychological harm.

Existing tools (CommentShark, BrandBastion, SmartModeration, Respondology) solve **brand safety and moderation** — hiding offensive content. They do **not** solve the **growth intelligence** problem: surfacing what truly matters to help creators grow faster and smarter.

### The Solution
**Noumena** is a B2C/B2B SaaS platform that connects to a creator's tracked channels, ingests all comments via platform APIs, and uses Large Language Models (LLMs) to classify, filter, and synthesize comments into **actionable growth insights**. It completely removes noise and psychological harm, and delivers curated, structured intelligence: top questions, audience pain points, viral content ideas, collaboration hooks, and product validation signals.

### Why Now?
- LLM API costs have dropped 10x in 18 months, making per-comment LLM processing economically viable at scale.
- Creator economy is at $250B+ globally (2025), with 200M+ professional/semi-professional creators who have no intelligence tooling.
- Platform APIs (YouTube Data API v3, TikTok Research API) are now stable and production-ready.
- Stripe's new metered billing makes consumption-based SaaS trivially implementable.

---

## 2. User Personas & Jobs-to-be-Done (JTBD)

### Persona 1 — The Growing Creator ("Alex")
- **Profile:** 50K–500K subscribers/followers. Full-time or transitioning to full-time. YouTube + Instagram or TikTok.
- **Core JTBD:** "When I publish a new video, I want to understand what my audience loved, hated, and wants next — without spending 3 hours reading comments or feeling devastated by trolls."
- **Secondary JTBD:** "When I'm planning my content calendar, I want validated ideas my audience has literally asked for."
- **Pain:** Manually moderates comments, misses key feedback, occasionally suffers from targeted hate campaigns. No structured feedback loop.
- **Willingness to Pay:** $29–$99/month.

### Persona 2 — The Creator Agency ("Growth Studio")
- **Profile:** Manages 5–50 creator accounts. Content strategists, social media managers.
- **Core JTBD:** "When managing multiple channels, I need a unified dashboard showing the health and growth opportunities of each creator's community — without reading thousands of comments per channel."
- **Secondary JTBD:** "When reporting to creators, I need data-backed content strategy recommendations."
- **Pain:** Spreadsheet-based reporting, patchwork of native tools, no cross-channel intelligence.
- **Willingness to Pay:** $199–$499/month for multi-seat access.

### Persona 3 — The Brand Collaborator ("Sponsor Desk")
- **Profile:** Brand marketing team evaluating creator partnerships.
- **Core JTBD:** "When vetting a creator for sponsorship, I want to understand the true sentiment and engagement quality of their community — beyond vanity metrics."
- **Pain:** Relies on follower counts and engagement rates that can be gamed. No comment quality signal.
- **Willingness to Pay:** $99–$299/month or per-report.

---

## 3. Functional Requirements (MoSCoW Prioritized)

### 3.1 Channel Management

> **Architectural note — Shared Channel Model:** A single real-world platform channel (e.g., a YouTube channel) can be tracked by **multiple independent Noumena users**. The system maintains one canonical `platform_channels` record per real channel, and one `user_channel_tracking` record per (user, platform_channel) pair. Comments are **ingested once** and shared across all users tracking that channel, saving API quota and LLM costs. OAuth tokens, tracking preferences, and starred comments are **per-user** and never shared.

| ID | Requirement | Priority | Acceptance Criteria |
|----|-------------|----------|---------------------|
| CM-01 | User can connect a YouTube channel via OAuth 2.0 | **Must** | After OAuth flow, system upserts a `platform_channels` record and creates a `user_channel_tracking` row with the user's encrypted tokens. If another user already tracks this channel, the platform channel record is reused and no new ingestion backfill is triggered (existing comments are already in the DB). |
| CM-02 | User can connect a TikTok account via OAuth 2.0 | **Should** | Same as CM-01 for TikTok Research API credentials. |
| CM-03 | User can connect an Instagram account via Meta Graph API | **Could** | Same as CM-01 for Meta credentials. |
| CM-04 | User can add tracked channels from the dashboard | **Must** | If the channel has never been tracked by anyone: triggers a backfill job. If already tracked by another user: the new user's tracking record is created instantly, with immediate access to all existing processed comments. |
| CM-05a | User can **stop tracking** a channel (pause, not delete) | **Must** | User's `user_channel_tracking.tracking_status` = `paused`. If other users still actively track the same channel, ingestion continues unaffected for them. The pausing user's dashboard no longer shows new comments from this channel, but all historical processed comments remain visible and searchable for that user. |
| CM-05b | User can **permanently remove** a tracked channel | **Must** | User's `user_channel_tracking` row is deleted. The user's starred comments for that channel are deleted. Shared `comments` and `platform_channels` are **not** deleted if any other user is still tracking it. If this was the **last** active tracker, the `platform_channels` record is scheduled for deletion and shared comments are purged within 24 hours. Irreversible — user must confirm via modal. |
| CM-06 | System notifies user when **their** OAuth token for a channel is about to expire (7 days before) | **Must** | Email notification sent at T-7 days for each affected `user_channel_tracking` row. In-app banner shown on login. Other users tracking the same channel are unaffected. |
| CM-07 | User can set a per-tracking comment backfill window (30/60/90/180 days) | **Should** | Applies only to first-time channel additions where no existing data is in the DB. Backfill job completes within 24 hours for up to 100K comments. |

### 3.7 Data Lifecycle

| ID | Requirement | Priority | Acceptance Criteria |
|----|-------------|----------|---------------------|
| DATA-01 | Stopping tracking a channel retains shared processed comments | **Must** | `user_channel_tracking.tracking_status` = `paused`. Shared `comments` rows are untouched. Re-activating resumes showing new comments to this user from `last_seen_at`. Ingestion for the channel continues if it has other active trackers. |
| DATA-02 | Shared comment data is only purged when the last tracker removes a channel | **Must** | System checks `COUNT(*) WHERE platform_channel_id = X AND tracking_status != 'removed'` before scheduling deletion. Purge is deferred until count = 0. |
| DATA-03 | Account deletion removes all user tracking relationships; shared comments are purged only if no other user tracks those channels | **Must** | All `user_channel_tracking` rows for the user are deleted. For each channel where this user was the last tracker, shared `comments` are hard-deleted. Stripe customer is canceled and archived. Confirmation email sent on request and on completion. |
| DATA-04 | User can export all their data before deleting account (GDPR Art. 20) | **Must** | Export includes: account metadata, all tracking relationships, all processed comments from channels they tracked (even if shared), all digest content, all starred comments. Delivered as a ZIP of JSON files within 24 hours via a secure, time-limited link. |

### 3.2 Comment Ingestion Pipeline

| ID | Requirement | Priority | Acceptance Criteria |
|----|-------------|----------|---------------------|
| ING-01 | System ingests new comments **per platform channel** (not per user) on a 30-minute poll interval | **Must** | One ingestion job per `platform_channels` record runs every 30 minutes, regardless of how many users track it. Comments are stored once and available to all trackers immediately. |
| ING-02 | System supports ingestion of video-level comments and top-level replies | **Must** | Both are stored with `parent_comment_id` to preserve thread structure. |
| ING-03 | System deduplicates comments across ingestion runs | **Must** | Re-ingesting the same `platform_comment_id` for the same platform channel does not create duplicate records. |
| ING-04 | Ingestion pipeline is fault-tolerant: failed API calls retry with exponential backoff (max 5 retries) | **Must** | Failed jobs retried at 1s, 2s, 4s, 8s, 16s. After 5 failures, job is dead-lettered and an alert sent to ops. |
| ING-05 | System respects platform API rate limits | **Must** | Rate limit headers are read and respected. Queue throttling applied automatically. |
| ING-06 | Only one ingestion job runs per platform channel at a time (serialized) | **Must** | SQLite serializes all writes, so per-channel mutual exclusion is guaranteed by claim atomicity: jobs are claimed via a single atomic `UPDATE … WHERE status='pending' … RETURNING`, and comment inserts dedupe with `ON CONFLICT DO NOTHING`. A dedicated advisory lock is not needed (and is a no-op by design). |
| ING-07 | Ingestion uses the OAuth token of the channel owner (one designated token per platform channel) | **Should** | If multiple users track the same channel, the system uses the token of the user who first connected it (the "primary token holder"). If that token is revoked or expired, the system automatically promotes the next active tracker's token. A `token_priority` field in `user_channel_tracking` controls ordering. |

### 3.3 LLM Comment Processing

| ID | Requirement | Priority | Acceptance Criteria |
|----|-------------|----------|---------------------|
| LLM-01 | System classifies each comment into one of 8 categories: Question, Praise, Criticism (Constructive), Criticism (Toxic), Content Idea, Collaboration Ask, Spam, Other | **Must** | Classification confidence score ≥ 0.7 accepted. Below 0.7 defaults to "Other". |
| LLM-02 | System extracts the primary topic/entity referenced in each comment | **Must** | Topic is a short string (≤ 10 words) stored in `comment.topic` field. |
| LLM-03 | System computes a sentiment score (−1.0 to +1.0) for each comment | **Must** | Score is stored with two decimal precision. |
| LLM-04 | System generates a weekly "Insight Digest" summarizing top trends across all tracked channels | **Must** | Digest is generated every Monday at 08:00 UTC. Contains top 5 content ideas, top 5 unanswered questions, sentiment trend, and engagement health score. |
| LLM-05 | Toxic/Spam comments are flagged and excluded from insight generation by default | **Must** | Comments classified as "Criticism (Toxic)" or "Spam" are not included in any digest or insight view unless user explicitly enables "show all" mode. |
| LLM-06 | User can customize the LLM classification prompt per channel (advanced mode) | **Could** | UI provides a text field to override the system prompt prefix. Custom prompt is validated (max 500 chars). |
| LLM-07 | System supports batch LLM processing: comments are batched in groups of 50 for cost efficiency | **Must** | Batch jobs are processed asynchronously. Per-comment cost is exposed in the billing dashboard. |
| LLM-08 | LLM provider is configurable (default: OpenAI GPT-4o, fallback: Anthropic Claude 3.5 Sonnet) | **Should** | Provider failover occurs automatically if primary provider returns >5 errors in 60 seconds. |

### 3.4 Insights Dashboard

| ID | Requirement | Priority | Acceptance Criteria |
|----|-------------|----------|---------------------|
| DASH-01 | Dashboard displays a real-time feed of high-signal comments (Questions, Content Ideas, Collaboration Asks) | **Must** | Feed updates within 60 seconds of LLM processing completing. Supports infinite scroll. |
| DASH-02 | Dashboard displays sentiment trend line (7d, 30d, 90d) per channel and across all channels | **Must** | Chart renders correctly for channels with ≥ 10 processed comments. |
| DASH-03 | User can filter comments by category, channel, date range, and sentiment score | **Must** | All filters are combinable. Filtered results update within 2 seconds (p95). |
| DASH-04 | User can "star" a comment for later review | **Should** | Starred comments appear in a dedicated "Saved" view. |
| DASH-05 | Dashboard displays a "Toxicity Shield" metric: % of comments filtered as toxic or spam | **Must** | Metric shown per channel and globally. |
| DASH-06 | User can export filtered comment views as CSV or JSON | **Should** | Export includes: comment text, category, sentiment, topic, author handle, timestamp. Generated within 60 seconds for up to 50K rows. |
| DASH-07 | Weekly Insight Digest is rendered in the dashboard and optionally emailed to the user | **Must** | Email delivery uses a transactional email provider. User can opt out. |

### 3.5 User Accounts & Authentication

| ID | Requirement | Priority | Acceptance Criteria |
|----|-------------|----------|---------------------|
| AUTH-01 | Users register with email + password or Google SSO | **Must** | Passwords hashed with bcrypt (cost factor ≥ 12). Google OAuth 2.0 PKCE flow. |
| AUTH-02 | Email verification required before accessing the product | **Must** | Unverified users can only see the "verify your email" screen. |
| AUTH-03 | Password reset via email link (expires in 1 hour) | **Must** | Reset link is single-use and HTTPS only. |
| AUTH-04 | Sessions expire after 30 days of inactivity | **Must** | JWT access tokens expire in 15 minutes; refresh tokens in 30 days with rolling window. |
| AUTH-05 | Multi-seat workspaces: Owner can invite team members with roles (Admin, Analyst, Viewer) | **Should** | Role-based access enforced server-side. Viewer cannot modify channel connections. |

### 3.6 Billing & Subscriptions (Stripe)

| ID | Requirement | Priority | Acceptance Criteria |
|----|-------------|----------|---------------------|
| BILL-01 | Free tier: 1 channel, up to 1,000 comments/month processed by LLM | **Must** | System enforces limit; processing paused when limit is reached. User shown upgrade prompt. |
| BILL-02 | Creator Plan ($29/month): 3 channels, 50,000 comments/month | **Must** | Limit resets on billing anniversary date. Overage: $0.50 per additional 1,000 comments. |
| BILL-03 | Studio Plan ($99/month): 15 channels, 250,000 comments/month | **Must** | Includes multi-seat (up to 5 users). |
| BILL-04 | Agency Plan ($299/month): Unlimited channels, 1,000,000 comments/month, 20 seats | **Must** | Custom overage pricing via sales. |
| BILL-05 | Stripe Checkout handles payment collection; no raw card data touches Noumena servers | **Must** | PCI compliance delegation to Stripe. Stripe Customer Portal used for subscription management. |
| BILL-06 | Webhook handler processes Stripe events: `checkout.session.completed`, `invoice.payment_failed`, `customer.subscription.deleted` | **Must** | Each event processed exactly once (idempotent via Stripe event ID). Subscription status reflected in DB within 30 seconds. |
| BILL-07 | Users receive email 3 days before trial expiry and on payment failure | **Must** | Emails sent via transactional provider. Payment failure triggers 3-day grace period before feature lock. |
| BILL-08 | Annual plans available at 20% discount | **Should** | Stripe Price IDs configured for monthly and annual intervals. |

---

## 4. Non-Functional Requirements

### 4.1 Performance
| NFR | Target |
|-----|--------|
| API response latency (p50) | ≤ 200ms |
| API response latency (p95) | ≤ 800ms |
| Dashboard page load (LCP) | ≤ 2.5s on 4G connection |
| LLM batch processing throughput | ≥ 500 comments/minute at steady state |
| Ingestion-to-insight latency | ≤ 60 minutes for new comments end-to-end |

### 4.2 Scalability
| NFR | Target |
|-----|--------|
| Concurrent active users supported | 10,000 (Phase 1), 100,000 (Phase 2) |
| Max comments stored per account | 10M (soft cap, extendable) |
| Database partitioning strategy | Partition `comments` table by `channel_id` hash |
| Horizontal scaling | All API and worker services stateless and horizontally scalable via container orchestration |

### 4.3 Security
| NFR | Requirement |
|-----|-------------|
| Data encryption at rest | AES-256 for all database storage |
| Data encryption in transit | TLS 1.3 minimum |
| OAuth tokens | Stored encrypted (AES-256) in database, never in client storage |
| OWASP Top 10 | All high-severity findings remediated before v1.0 launch |
| Rate limiting | API endpoints rate-limited per user: 100 req/min (authenticated), 10 req/min (unauthenticated) |
| GDPR compliance | User data deletion within 30 days of account deletion request. Data export on demand. |
| Dependency auditing | `go mod audit` and `npm audit` run in CI on every PR |

### 4.4 Availability & Reliability
| NFR | Target |
|-----|--------|
| SLA uptime | 99.5% monthly (≤ 3.6 hours downtime/month) |
| RTO (Recovery Time Objective) | 4 hours |
| RPO (Recovery Point Objective) | 1 hour |
| Database backups | Daily SQLite backup (`sqlite3 .backup` after a WAL checkpoint), N-copy rotation |

---

## 5. User Flows & Edge Cases

### Flow 0: User Registration, Login & Account Recovery

#### 0a — Registration via Email & Password

**Happy Path:**
1. User lands on `/signup` → enters email + password (min 8 chars, 1 uppercase, 1 number).
2. System validates email format and password strength in real time (client-side).
3. On submit, API checks email uniqueness → hashes password with bcrypt (cost ≥ 12) → creates `users` row with `email_verified_at = NULL`.
4. System sends a verification email containing a one-time signed link (expires in 24 hours).
5. User is redirected to a `/verify-email` holding page: "Check your inbox to verify your account."
6. User clicks the link → API validates the token → sets `email_verified_at = NOW()` → redirects to onboarding wizard (Flow 2).

**Edge Cases:**
- **EC-0a.1 Email already registered (password account):** API returns 409. UI shows: "An account with this email already exists. [Log in instead]."
- **EC-0a.2 Email already registered via Google SSO:** API returns 409 with hint. UI shows: "This email is linked to a Google account. [Continue with Google]."
- **EC-0a.3 Verification link expires (24h):** User clicks expired link → API returns 410 → UI shows: "This link has expired. [Resend verification email]." A fresh token is generated on resend. Old token is invalidated.
- **EC-0a.4 User tries to access the product before verifying email:** All authenticated routes redirect to `/verify-email` with a banner. Session is valid but product access is gated until `email_verified_at` is set.
- **EC-0a.5 User registers but never verifies — account is inactive for 7 days:** A scheduled cleanup job deletes unverified accounts older than 7 days to prevent email enumeration attacks.

---

#### 0b — Registration & Login via Google SSO

**Happy Path:**
1. User clicks "Continue with Google" → redirected to Google OAuth 2.0 consent screen (PKCE flow).
2. User grants consent → system receives authorization code → exchanges for ID token.
3. System extracts `sub` (Google user ID) and email from ID token:
   - **New user:** Creates `users` row with `google_sub`, `email_verified_at = NOW()` (Google emails are pre-verified). Redirects to onboarding wizard.
   - **Returning user:** Looks up `users.google_sub` → issues JWT access + refresh tokens → redirects to dashboard.
4. No email verification step required (Google guarantees email ownership).

**Edge Cases:**
- **EC-0b.1 Google account email matches an existing password account:** System links the Google SSO to the existing account (sets `google_sub`) after prompting: "We found an existing account with this email. [Link your Google account]?" Password remains valid.
- **EC-0b.2 User denies Google consent:** System returns to `/signup` with message: "Google sign-in was cancelled. You can sign up with email instead."
- **EC-0b.3 Google returns an invalid or tampered ID token:** API verifies token signature against Google's JWKS endpoint. If verification fails, request is rejected with 401 and the flow is aborted entirely.

---

#### 0c — Login (Email & Password)

**Happy Path:**
1. User navigates to `/login` → enters email + password → submits.
2. API fetches user by email → compares bcrypt hash → on success: issues JWT access token (15-min TTL) + refresh token (30-day rolling) → stores refresh token hash in DB.
3. User is redirected to `/dashboard`.
4. Frontend stores access token in memory (Pinia store). Refresh token is stored in an `HttpOnly`, `Secure`, `SameSite=Strict` cookie.
5. On subsequent requests, if the access token is expired, the frontend silently calls `/auth/refresh` → receives a new access token without re-login.

**Edge Cases:**
- **EC-0c.1 Email not found:** API returns 401 with a generic message: "Invalid email or password." (No email enumeration — do not specify which field is wrong.)
- **EC-0c.2 Correct email, wrong password (brute-force protection):** After 5 failed attempts within 15 minutes, the account is temporarily locked for 15 minutes. User receives an email: "Multiple failed login attempts detected. Your account is locked for 15 minutes."
- **EC-0c.3 User logs in with correct credentials but email is unverified:** Login succeeds (JWT is issued) but all product routes redirect to `/verify-email`. A "Resend verification" link is shown.
- **EC-0c.4 Refresh token is expired or revoked:** `/auth/refresh` returns 401. Frontend clears session state and redirects user to `/login` with message: "Your session has expired. Please log in again."
- **EC-0c.5 User is logged in on multiple devices:** Each device holds its own refresh token. Revoking one device (e.g., "Log out of all devices" in settings) invalidates all refresh tokens for that user by incrementing a `token_version` field in `users`. All outstanding tokens with an older version return 401.

---

#### 0d — Password Reset

**Happy Path:**
1. User clicks "Forgot password?" on `/login` → enters their email.
2. API always returns: "If an account exists with this email, a reset link has been sent." (no enumeration).
3. If email exists: system generates a one-time reset token (cryptographically random, SHA-256 hashed in DB) → sends email with a link valid for 1 hour.
4. User clicks the link → lands on `/reset-password` → enters and confirms a new password.
5. API validates token (not expired, not already used) → updates `password_hash` → marks token as used → invalidates all existing refresh tokens (force re-login on all devices) → redirects to `/login` with: "Password updated successfully."

**Edge Cases:**
- **EC-0d.1 Reset link is expired (>1 hour):** API returns 410. UI shows: "This link has expired. Request a new one." The old token is deleted.
- **EC-0d.2 Reset link is used a second time:** Token is marked `used = true` on first use. Second attempt returns 410: "This reset link has already been used."
- **EC-0d.3 User requests multiple reset emails in quick succession:** Rate-limited to 3 reset emails per hour per email address. Further requests return the same generic success message but no email is sent. Prevents email bombing.
- **EC-0d.4 User has a Google SSO account with no password:** `/forgot-password` detects `password_hash IS NULL` → sends email: "Your account uses Google Sign-In. To set a password, log in with Google and go to Account Settings."

---

### Flow 1: First-Time Onboarding & Channel Connection

**Happy Path:**
1. User completes registration (Flow 0a or 0b) → lands on onboarding wizard.
2. Clicks "Connect YouTube Channel" → redirected to Google OAuth consent screen.
3. Grants read-only access → redirected back to Noumena with authorization code.
4. System exchanges code for tokens → stores encrypted tokens → creates `user_channel_tracking` record (and `platform_channels` if new).
5. Background job triggers 90-day comment backfill.
6. User sees "Syncing your channel... this may take up to 24 hours for large channels."
7. When backfill is ≥ 10% complete, dashboard shows first insights.

**Edge Cases:**
- **EC-1.1 User denies OAuth permissions:** System returns user to channel connection page with error banner: "Permission denied. Please grant read access to continue." No partial records created.
- **EC-1.2 User connects a channel with no videos:** System successfully connects but backfill returns 0 comments. Dashboard shows empty state: "No videos found on this channel. Try adding a channel where you publish content."
- **EC-1.3 Google OAuth token expires during backfill:** Ingestion worker detects 401, triggers token refresh. If refresh also fails (user revoked access), job is paused and user receives an in-app notification to reconnect the channel. No data in flight is lost (job resumes from last checkpoint).

---

### Flow 2: Weekly Insight Digest Generation

**Happy Path:**
1. Cron job triggers every Monday 08:00 UTC.
2. For each active user, digest job fetches all comments processed in the last 7 days.
3. Comments classified as Toxic/Spam are excluded.
4. LLM summarization call generates: top 5 content ideas, top 5 unanswered questions, overall sentiment trend, engagement health score.
5. Digest stored in `weekly_digests` table, rendered in dashboard, and emailed if opted in.

**Edge Cases:**
- **EC-2.1 User has fewer than 10 processed comments in the last 7 days:** Digest is not generated. Dashboard shows: "Not enough comment data this week. Post more content or lower your comment filter thresholds." No failed email attempt.
- **EC-2.2 LLM API is unavailable during digest generation:** Job retries up to 3 times over 2 hours. If all fail, user receives email: "Your weekly digest is delayed. We'll deliver it when our AI service recovers." Digest is marked `deferred` in DB.
- **EC-2.3 User changes their channel list mid-week (removes a channel):** Digest is computed only over channels active at the time of digest generation. Removed-channel comments are excluded.

---

### Flow 3: Payment & Subscription Upgrade

**Happy Path:**
1. User on Free tier hits 1,000 comment limit.
2. Dashboard banner: "You've used 100% of your monthly AI credits. Upgrade to keep insights flowing."
3. User clicks "Upgrade" → Stripe Checkout opens (pre-filled with email).
4. User selects Creator Plan → enters payment details → confirms.
5. Stripe sends `checkout.session.completed` webhook.
6. Noumena webhook handler updates `user.subscription_plan` to `creator` and `user.comment_quota` to 50,000.
7. User is redirected to dashboard with success banner. Processing resumes immediately.

**Edge Cases:**
- **EC-3.1 Stripe webhook is received before Checkout redirect returns to app:** System handles this gracefully via Stripe event idempotency key. Subscription state is correct regardless of webhook vs. redirect order.
- **EC-3.2 User's payment is declined:** Stripe retries 3 times over 3 days. Noumena receives `invoice.payment_failed` event and displays persistent in-app banner. After 3-day grace period, processing is paused (data still stored). Account is not deleted.
- **EC-3.3 User downgrades from Studio to Creator plan:** Downgrade takes effect at end of current billing period. If the user has 10 channels connected but Creator plan allows 3, excess channels are queued but not deleted. On plan effective date, ingestion pauses for channels beyond the 3-channel limit. User receives notification listing which channels are paused and prompted to select active channels.
- **EC-3.4 Duplicate Stripe webhook delivery:** Handler checks `stripe_event_id` against `processed_stripe_events` table. Duplicate is acknowledged with 200 OK but not re-processed.

---

### Flow 4: Comment Filtering & Insight Exploration

**Happy Path:**
1. User opens Dashboard → sees "High-Signal Feed" with latest Questions and Content Ideas.
2. User applies filter: Channel = "My Photography Channel", Category = "Question", Date = "Last 30 days."
3. System returns filtered result set within 2 seconds.
4. User stars a comment → it appears in "Saved" view.
5. User exports filtered results as CSV.

**Edge Cases:**
- **EC-4.1 Filter combination returns 0 results:** System displays: "No comments match your current filters. Try expanding the date range or selecting additional categories." No error state.
- **EC-4.2 Export for large result sets (>50K rows):** System queues export job, notifies user via email with download link (expires in 24 hours). In-app notification also shown.
- **EC-4.3 User tries to view unprocessed comments:** Comments ingested but not yet LLM-processed are shown with a "Processing..." badge in the feed but are excluded from all aggregated insights until processing completes.

---

### Flow 5: Stopping Channel Tracking

**Happy Path:**
1. User opens Channel Settings → clicks "Pause Tracking" on the target channel.
2. System presents confirmation modal: "Pausing will stop this channel from appearing in your feed. Other users tracking this channel are not affected, and shared comment data is preserved."
3. User confirms → `user_channel_tracking.tracking_status` = `paused` for this user only.
4. System checks: does this channel have other active trackers? **Yes** → shared ingestion continues unaffected. **No** → ingestion worker stops polling this platform channel (but comments already in DB are retained).
5. Channel appears in the user's dashboard with a "Paused" badge. All historical processed comments remain fully accessible and searchable for this user.
6. Weekly Digest for this user excludes paused channels.
7. To resume: user clicks "Resume Tracking" → `tracking_status` reset to `active` → new comments (ingested after the pause) immediately appear in this user's feed; no reprocessing needed since they were already in the shared DB.

**Edge Cases:**
- **EC-5.1 User pauses a channel mid-backfill:** The in-flight backfill batch completes, then the user's tracking is paused. If no other users track this channel, no further ingestion batches are enqueued. Any comments already processed are retained and remain visible to the pausing user.
- **EC-5.2 User is the last tracker and pauses:** Shared ingestion worker stops polling the platform channel. On resume, the worker catches up from `platform_channels.last_synced_at` — comments published during the pause that were not ingested will be fetched in the next ingestion cycle.
- **EC-5.3 User pauses a channel, another user then connects the same channel:** The new user's `user_channel_tracking` row is created as `active`. Ingestion restarts for the platform channel. When the original user resumes, all comments ingested for the other user are immediately visible to them (already processed, no cost).
- **EC-5.4 Paused user's OAuth token is the primary ingestion token and it expires:** System promotes the next `token_priority` tracker's token automatically (ING-07). The paused user sees a notification only on resume, prompting them to reconnect. Other users are not interrupted.

---

### Flow 6: Account Deletion

**Happy Path:**
1. User navigates to Account Settings → "Delete Account".
2. System presents a two-step confirmation:
   - Step 1: "This will permanently remove all your tracked channels, personal insights, and account data. Shared comment data will only be deleted for channels where you are the sole tracker." + button: "I understand, request deletion."
   - Step 2: User must type their email address to confirm.
3. On confirmation, system:
   a. Immediately locks the account (sets `users.deletion_requested_at = NOW()`). User is signed out and cannot sign back in.
   b. If user has an active paid subscription: Stripe subscription is canceled immediately. `customer.subscription.deleted` event is processed.
   c. A GDPR data export ZIP is generated and emailed to the user (link valid for 7 days) as a courtesy.
   d. Enqueues an async `account_deletion` job.
4. Async deletion job (runs within 30 days, typically within 24 hours):
   a. For each `user_channel_tracking` row belonging to the user:
      - Revokes the user's OAuth token via the platform API.
      - Checks remaining active tracker count for the `platform_channel_id`.
      - If **other active trackers exist**: only the `user_channel_tracking` row is deleted; shared `comments` and `platform_channels` record are retained.
      - If **this user was the last tracker**: shared `comments` are hard-deleted (chunked by 10K rows), and the `platform_channels` record is deleted.
   b. Deletes all `user_comment_stars` rows for the user.
   c. Hard-deletes all `weekly_digests` rows for the user.
   d. Hard-deletes the `users` row.
   e. Stripe customer record is **not** deleted (retained by Stripe for legal/financial record-keeping).
5. A final confirmation email is sent to the user's address: "Your Noumena account and all associated data have been deleted."

**Edge Cases:**
- **EC-6.1 User requests deletion then tries to sign in:** Account is locked immediately. Login returns: "This account has been scheduled for deletion." with a link to contact support (reversible within 48 hours if purge has not yet run).
- **EC-6.2 OAuth token revocation fails for one platform during deletion:** Deletion job logs the failure and continues. Background retry attempts revocation 3 times over 6 hours. Tokens have read-only scope so residual risk is minimal. Account deletion proceeds regardless.
- **EC-6.3 User was the sole tracker of a channel with >1M comments:** Deletion job uses chunked hard-deletes (10K rows/transaction) scoped to that `platform_channel_id`. Progress is tracked in `account_deletion_jobs` so it can resume if interrupted. Other channels belonging to other users are not touched.
- **EC-6.5 User deletes account while being the primary token holder (ING-07) for a channel that other users still track:** Before deleting the user's token, the system promotes the next eligible tracker's token to primary. Ingestion continues uninterrupted for the remaining users.
- **EC-6.4 User deletes account during an active Annual plan:** System cancels the Stripe subscription at period end by default (preventing future charges). The user retains access until the paid period expires, then the deletion job runs. The user is notified of this timeline explicitly in the confirmation modal.

---

## 6. Technology Stack & Architecture

### 6.1 Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| **Backend API** | Go (Golang) | High performance, low memory footprint, excellent concurrency primitives for ingestion workers. |
| **Database** | SQLite (embedded, `modernc.org/sqlite`) | Zero-ops, single-file; WAL journaling + FK + busy_timeout; JSON stored as TEXT; full-text via FTS5. |
| **Frontend** | Vue 3 (Composition API) + Vite | Component-driven, reactive UI. Excellent TypeScript support. |
| **UI Styling** | Tailwind CSS | Utility-first for rapid UI development; design system enforced via `tailwind.config`. |
| **Payments** | Stripe (Checkout + Billing + Webhooks) | Industry standard; eliminates PCI scope for raw card data. |
| **LLM Processing** | OpenAI GPT-4o (primary), Anthropic Claude 3.5 Sonnet (fallback) | GPT-4o offers best cost/performance for classification; Claude as fallback for resilience. |
| **Job Queue** | SQLite-backed queue (atomic `UPDATE … RETURNING` claim) | Avoids Redis dependency; transactional enqueue; single-writer model. |
| **Email** | Resend (transactional) | Developer-friendly, excellent deliverability, Vue email template support. |
| **Auth** | Custom JWT (Go) + Google OAuth 2.0 | Full control, no vendor lock-in on auth. |
| **Deployment** | Docker + Docker Compose (dev), VPS/Cloud (prod) | Simple containerization; horizontally scalable. |

### 6.2 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                          Vue 3 Frontend                          │
│              (Dashboard, Onboarding, Billing Portal)             │
└───────────────────────────┬─────────────────────────────────────┘
                            │ HTTPS / REST API
┌───────────────────────────▼─────────────────────────────────────┐
│                        Go REST API                               │
│  /auth  /channels  /comments  /insights  /billing  /webhooks     │
└──────┬───────────────────────────────────────┬──────────────────┘
       │                                       │
┌──────▼──────────┐                  ┌────────▼────────────────┐
│      SQLite      │                  │    Background Workers    │
│   (embedded)     │◄─────────────────│  Ingestion | LLM Batch  │
│  - users         │                  │  Digest Gen | Webhooks   │
│  - platform_chs  │                  └────────────┬────────────┘
│  - tracking      │                               │
│  - comments      │                  ┌────────────▼────────────┐
│  - digests       │                  │    External Services      │
│  - job_queue     │                  │  YouTube API | TikTok API │
└─────────────────┘                  │  OpenAI | Anthropic       │
                                     │  Stripe | Resend (email)  │
                                     └───────────────────────────┘
```

### 6.3 Database Schema (Core Tables)

The canonical schema lives in [`internal/db/schema.sql`](internal/db/schema.sql) — SQLite
dialect (`modernc.org/sqlite`), applied idempotently (`CREATE … IF NOT EXISTS`) at startup.
Core tables:

| Table | Purpose |
|-------|---------|
| `users` | Auth (bcrypt hash, `google_sub`), plan, quota counters, `token_version` |
| `platform_channels` | One row per real YouTube channel, shared across users |
| `user_channel_tracking` | Per-user OAuth tokens + tracking preferences per channel |
| `channel_grants` | Workspace → channel sharing grants |
| `comments` | Ingested comments + LLM classification (category, sentiment, topic, confidence) |
| `comments_fts` | FTS5 virtual table over `comments.body` (kept in sync via triggers) |
| `user_comment_stars` | Per-user starred comments |
| `weekly_digests` | Per-user AI digests (`content` stored as JSON TEXT) |
| `ingestion_jobs`, `llm_processing_jobs` | SQLite-backed job queues |
| `account_deletion_jobs`, `data_export_jobs` | Async lifecycle jobs |
| `email_send_log` | SMTP idempotency + retry ledger |
| `rate_limit_buckets`, `workspaces`, `workspace_members`, `workspace_invites`, `audit_log`, `oauth_states`, `refresh_tokens`, `processed_stripe_events` | Supporting infrastructure |

---

## 7. Success Metrics (KPIs)

### 7.1 Acquisition & Activation
| Metric | Target (Month 3) | Target (Month 12) |
|--------|-----------------|------------------|
| Monthly Signups | 500 | 5,000 |
| Activation Rate (connected ≥1 channel within 24h) | ≥ 60% | ≥ 70% |
| Time to First Insight (from signup) | ≤ 2 hours | ≤ 1 hour |

### 7.2 Retention & Engagement
| Metric | Target |
|--------|--------|
| Week-1 Retention | ≥ 50% |
| Month-1 Retention | ≥ 35% |
| Weekly Digest Open Rate (email) | ≥ 40% |
| Dashboard DAU/MAU ratio | ≥ 25% |

### 7.3 Revenue
| Metric | Target (Month 6) | Target (Month 12) |
|--------|-----------------|------------------|
| Monthly Recurring Revenue (MRR) | $10,000 | $50,000 |
| Free → Paid Conversion Rate | ≥ 5% | ≥ 8% |
| Average Revenue Per User (ARPU) | ≥ $45 | ≥ $60 |
| Churn Rate (monthly) | ≤ 5% | ≤ 3% |

### 7.4 Product Quality
| Metric | Target |
|--------|--------|
| LLM Classification Accuracy (vs. human labels) | ≥ 85% |
| Comment Ingestion Success Rate | ≥ 99% |
| API p95 Latency | ≤ 800ms |
| NPS (Net Promoter Score) | ≥ 40 |
| Support Tickets per 100 Active Users | ≤ 2 |

---

## 8. Out of Scope (Won't Do — v1.0)

- Real-time comment stream (WebSocket); polling-based ingestion is sufficient for v1.
- AI-generated comment reply suggestions (planned for v1.5).
- Native mobile app (web-first; PWA will be evaluated in v1.2).
- Twitch, LinkedIn, X/Twitter comment ingestion (v2.0 backlog).
- Human moderation review queue (not our differentiation; focus is intelligence, not moderation).
- Custom LLM fine-tuning (evaluated at 10M+ comments processed).

---

## 9. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| YouTube API quota limits (10,000 units/day per project) | High | High | Request quota increase from Google. Implement smart polling (skip channels with no recent activity). Offer API-key-sharing model for high-volume users. |
| LLM API costs exceed revenue at scale | Medium | High | Batch processing (50 comments/call). Implement result caching for near-duplicate comments. Set hard per-tenant cost limits. Monitor unit economics weekly. |
| TikTok Research API access revoked / restricted | Medium | Medium | TikTok is a "Should" priority platform. Build abstraction layer so removing TikTok doesn't affect core product. |
| Creator churn after initial "wow" wears off | Medium | High | Weekly Digest creates a recurring reason to log in. Invest in email lifecycle (onboarding, digest, achievement milestones). |
| Stripe SCA compliance for European users | Low | Medium | Stripe Checkout handles SCA natively. Test with 3DS-required test cards during QA. |

---

## 10. Open Questions (for Engineering Kickoff)

1. **LLM batching strategy:** Should we batch by channel (all comments from one channel) or globally (cross-channel)? Global batching is more cost-efficient but requires care to avoid cross-user data leakage in the prompt context.
2. **Job queue mechanics:** SQLite-backed queue with atomic `UPDATE … WHERE status='pending' … RETURNING` claim semantics; per-channel exclusion relies on SQLite's write serialization. No external queue infrastructure (Redis, SQS). [Resolved]
3. **Multitenancy isolation level:** Application-level access-control joins at the SQL layer (mandatory on every data route); workspace role enforcement lands with the multi-seat workspaces change. [Resolved]
4. **Frontend auth state management:** Pinia store with JWT in memory (preferred) vs. localStorage (XSS risk). Confirm with security-conscious approach.
5. **Comment partitioning:** Hash partitioning by `channel_id` (even distribution) vs. range partitioning by `posted_at` (better for time-based queries). Recommend hybrid: partition by `channel_id` hash, add composite index `(channel_id, posted_at)`.

---

*End of PRD v1.3 — Noumena*

> **Verification Gate Checklist (Self-QA) — v1.3:**
> - [x] No "TBD" sections present.
> - [x] All functional requirements are measurable/testable via acceptance criteria.
> - [x] Minimum 3 edge cases per major feature (7 flows, 30+ total edge cases documented).
> - [x] NFRs cover Scalability, Security, Performance, and Availability.
> - [x] Tech stack aligns with user-specified choices: Go, SQLite, Vue 3, Tailwind CSS, Stripe.
> - [x] Risks documented with mitigations.
> - [x] Open questions flagged for engineering team.
> - [x] Full auth lifecycle documented in Flow 0: registration (email + Google SSO), login, session management, password reset.
> - [x] Shared channel model: `platform_channels` + `user_channel_tracking` split (CM-01, CM-04, ING-01, ING-06, ING-07).
> - [x] Comments ingested once per platform channel, shared across all trackers — no duplicate API/LLM cost.
> - [x] Pause/remove semantics respect other active trackers before touching shared data (DATA-01, DATA-02).
> - [x] Account deletion only purges shared comments where user was the last tracker (DATA-03, EC-6.3).
> - [x] Primary ingestion token promoted automatically on pause or account deletion (ING-07, EC-6.5).
> - [x] Starred comments are per-user (`user_comment_stars` table), never shared.
