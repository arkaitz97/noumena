# Design

Status: current. Scope: the backend and dashboard in this repository.
Owner: Arkaitz Molinuevo. Last reviewed: 2026-11-24.

## 1. Context and scope

Content creators receive more comments than they can read. The valuable
fraction is small: recurring questions, content ideas, collaboration requests,
product-validation signals, and early signs of a hostile thread. Noumena
collects comments from a creator's own channels, classifies them with a large
language model, and presents the classified feed, aggregates, digests, and
exports.

The system is a single Go binary (`cmd/api`) plus a Vue 3 single-page app
(`frontend/`). The binary serves the HTTP API and runs the background workers
in goroutines: scheduler, ingestion worker, LLM processor, digest worker,
export worker, lifecycle workers, quota cron, trial reminder, overage
invoicing. Persistence is SQLite in WAL mode (`internal/db/db.go`,
`modernc.org/sqlite`). Stripe handles subscriptions. One LLM provider is
primary and another is optional fallback.

In scope:

- OAuth connection of a creator's YouTube channel.
- Scheduled or backfilled ingestion of comments and replies.
- Batch classification, sentiment, and insight extraction.
- Feed, search, saved comments, aggregates, weekly digests, CSV/JSON export.
- Subscription billing, metered overage, plan limits, seat limits.
- Account lifecycle: verification, password reset, data export, deletion.

Out of scope (code exists only as a seam or not at all):

- Platforms other than YouTube. The compliance review in
  `docs/compliance/platform-tos-compliance.md` explains why TikTok,
  Instagram, and X are blocked or gated.
- Multi-region deployment, read replicas, and horizontal write scale. SQLite
  is the system of record.
- A hosted multi-tenant SaaS operation. The repository is a personal project.
- Payment methods other than Stripe.

## 2. Goals and non-goals

Goals, each with a measure in [drivers.md](drivers.md):

1. Never show a user a comment from a channel they cannot access. (S-2)
2. Survive one LLM provider outage without losing queued work. (S-1)
3. Make every ingestion retry and every Stripe redelivery idempotent. (S-3, S-6)
4. Erase personal data on request, including files on disk. (S-4)
5. Keep the operating cost proportional to usage: LLM calls are batched,
   ingestion respects API budgets, and overage is metered. (C-1, O-1)

Non-goals:

- Real-time comment streaming. Polling at an adaptive interval is enough.
- Per-tenant physical database separation. Isolation is a query predicate plus
  row ownership, not a database per customer.
- Exactly-once LLM classification. At-least-once with an idempotent upsert is
  the contract.
- Horizontal scale. Two API processes would break the worker claim model.

## 3. The design

### 3.1 Request path

`chi` router. Middleware order in `cmd/api/main.go`:

1. Access log (path only, never the query string).
2. OpenTelemetry span and metrics.
3. Panic recovery.
4. Security headers and a 2 MiB body cap.
5. `/api` group: JWT authentication, verified-email gate, per-user rate limit,
   workspace resolution.
6. Workspace routes: `WorkspaceMiddleware` loads the membership row and puts
   the role in context. `RequireWorkspaceRole` gates writes to `analyst` and
   `admin`.

Access tokens are 15-minute HS256 JWTs carrying a token version
(`internal/auth/jwt.go`). Refresh tokens are opaque, hashed at rest, rotated on
use, and revoked by version bump (`refresh_tokens` table). Provider tokens are
AES-GCM encrypted with a versioned key ID so a key can rotate without a
migration (`internal/auth/encryption.go`).

### 3.2 Channel and comment model

Comments belong to a `platform_channels` row, not to a user. A channel is
identified by `(platform, platform_channel_id)`. Several users may track the
same channel; each has a `user_channel_tracking` row with its own OAuth token
and backfill window. Comments are stored once and shared.

Visibility is one SQL predicate in `accessControlClause`
(`internal/db/comments_feed.go`):

```sql
EXISTS (tracking row for $1 with status active or paused)
OR EXISTS (channel_grants row joined to a workspace_members row for $1)
```

Every comment read path routes through `BuildFeedQuery`: feed, saved, count,
and the export stream. This is deliberate. A new read path cannot forget the
predicate because the predicate is emitted unconditionally by the builder.

The workspace is a collaboration overlay, not a billing tenant. Billing,
quota, and deletion live on `users`. A workspace shares channels through
`channel_grants` and enforces seats from the owner's plan
(`internal/api/workspace_handlers.go`).

### 3.3 Ingestion

The scheduler (`internal/ingestion/scheduler.go`) scans channels due for a
sync and enqueues a `poll` or `backfill` job. A pending or running job for the
channel suppresses a duplicate enqueue. The first sync and any incomplete
backfill use `backfill`; later runs use `poll`.

The worker (`internal/ingestion/worker.go`) claims one job, loads a usable
tracking token, discovers videos from the uploads playlist, walks comment
threads, and writes in batches of 100. It enforces per-job ceilings: 100
videos, 25 thread pages, 100 reply fetches. Exceeding a ceiling is a clean
stop, not a failure; the job is rescheduled and the checkpoint holds the
position.

Two checkpoint levels exist:

- Channel phase: `channel_sync_checkpoints` with `page_token`, `boundary`, and
  status (`pending`, `in_progress`, `paused`, `complete`).
- Per-video: `video_sync_states` with a watermark and a page checkpoint.

A batch of comments and the matching checkpoint advance commit in one
transaction (`CommitCommentsAndCheckpoint`). Comment insert uses
`ON CONFLICT (platform_channel_id, platform_comment_id) DO UPDATE`, so a
replayed page updates video metadata and does not duplicate rows.

Backfill windows are 30, 60, 90, or 180 days (`backfillDayOptions`). Changing
the window changes the horizon of the next backfill. It does not re-fetch
history that is already stored.

Failures: 5 attempts with backoff, then dead-letter. A dead-letter invokes an
alerter; the default logs, the configured option posts JSON to a webhook. An
invalid token promotes the next tracker's token and requeues without consuming
the retry budget.

### 3.4 Classification

Comment rows are queued in `llm_processing_jobs` and processed in batches of
15 with up to 4 concurrent workers. The prompt is closed-set: category,
confidence, topic, sentiment, suggested reply, and typed actionable insights
(`internal/llm/prompts.go`). Categories:
`question`, `praise`, `constructive_criticism`, `toxic_criticism`,
`content_idea`, `collaboration_ask`, `spam`, `other`. Results below confidence
0.7 become `other`.

`failoverClient` wraps the primary and optional fallback provider. Three
consecutive primary errors flip the route permanently for the process
lifetime. A success resets the counter. Retries are 1, 5, and 15 minutes, up
to 3 attempts, with the attempt state persisted on the comment row.

### 3.5 Billing

The plan catalog (`internal/billing/plans.go`) is the only source of limits:
free 1 channel/1,000 comments, creator 3/50,000, studio 15/250,000 with 5
seats, agency unlimited/1,000,000 with 20 seats. Stripe holds the price;
the catalog echoes it.

Checkout attaches the user ID as `client_reference_id`. The webhook is the
authority for subscription state. `WebhookStripe` verifies the signature,
reads at most 1 MiB, checks `processed_stripe_events` by event ID, then, when
a pool is present, applies the side effects and inserts the processed-event
row in one transaction. A duplicate event returns 200 without dispatch.
Unhandled event types are marked processed so Stripe stops retrying.

Free-tier quota resets daily at 08:00 UTC; past-due accounts get a 72-hour
grace; overage invoicing runs at 08:10 UTC from `comment_usage_logs`.

### 3.6 Deletion and export

`DELETE /api/user/account` requires the account email as confirmation, marks
`deletion_requested_at`, cancels the Stripe subscription, and enqueues an
`account_deletion_jobs` row. The worker waits out the reversible window
(default 720 hours = 30 days), generates a GDPR export, then deletes.

Per channel the worker revokes OAuth tokens and removes the user's tracking
row and stars. If another user still tracks the channel, shared comments
survive and the next tracker is promoted to primary. If no tracker remains,
comments are deleted in chunks and the channel row is dropped. Finally the
worker removes the export directory from disk, wipes 15 user-scoped tables,
sends the completion email, deletes the email log rows, and hard-deletes the
user in a separate transaction.

## 4. Alternatives considered

| Alternative | Why rejected |
|---|---|
| PostgreSQL with `pgx` and Redis | The workload is one writer and a handful of workers. SQLite in WAL mode removes two services and their failure modes. Revisit when writes or concurrent readers exceed one node. |
| Database per tenant | Strong isolation, but it multiplies migrations, backups, and connection handling. The shared store with one access predicate plus row-level grants is enough for the current trust model. |
| Webhook or PubSubHubbub ingestion | YouTube push notifications do not carry comment payloads, so a fetch is still required. Polling with checkpoints is simpler and survives downtime. |
| Per-user comment copies | Would simplify deletion but multiply storage and LLM cost for shared channels. The shared store plus detach-or-purge deletion keeps cost linear in distinct channels. |
| Publish events to a queue (Kafka/SQS) between API and workers | The API and workers share a process and a database. A table-backed queue with atomic claim is the same semantics with one less system. |
| Store a raw transcript of every LLM call | Useful for debugging, but it duplicates comment PII and widens the GDPR surface. Only derived fields are stored. |
| Trust the Stripe object in the webhook | Older events can arrive after newer ones. The handler fetches the authoritative subscription where the event carries a checkout session, and derives state from event type plus current object. |

## 5. Cross-cutting concerns

### Security

- Tenant isolation is a mandatory predicate, tested in S-2.
- Passwords use bcrypt; reset tokens are hashed; login lockout counts failed
  attempts. JWT secret must be at least 32 bytes and is validated at boot.
- OAuth provider tokens are AES-GCM encrypted; a previous key is accepted for
  rotation. Tokens never appear in logs.
- Webhook bodies are capped at 1 MiB and the signature is verified before any
  parsing side effect. All other bodies are capped at 2 MiB.
- Security headers: HSTS on HTTPS, `nosniff`, frame deny, no referrer, and a
  restrictive CSP. Access logs omit query strings so reset tokens cannot leak.
- Rate limits: 100 requests per minute for authenticated users, 10 for
  anonymous, always on for auth endpoints.
- `gitleaks` runs on push and pull request over the full history.

### Privacy and GDPR

- Personal data is user-scoped and enumerated in `DeleteUserGDPRData`.
- Deletion is reversible for 30 days, then irreversible. The UI and email state
  the deadline.
- Shared comment bodies are third-party content, not the deleting user's data.
  The worker removes the user's tracking rows, stars, exports, and tokens, and
  keeps shared rows only while another tracker needs them.
- Export links are signed and expire after 7 days. Export files are removed
  before the database wipe, and a failure aborts the deletion rather than
  orphaning PII.
- `docs/compliance/platform-tos-compliance.md` records the platform-level
  retention limits that constrain what may be stored at all.

### Cost

- LLM cost is bounded by the plan quota, batch size, and a per-user monthly
  cost rollup (`internal/llm/cost.go`). Overage is metered, not silent.
- Ingestion cost is bounded by the per-job ceilings and an adaptive poll
  interval that slows down quiet channels.
- Storage cost is bounded by SQLite on one volume and a 24-hour export TTL.
- The biggest cost risk is a retry storm against a failing provider. Failover
  and dead-lettering exist to cap it.

## 6. Known risks and gaps

- SQLite serializes writes. One long export read uses a separate read-only
  handle to avoid holding the main connection, but a write burst still queues.
- The deletion default is 720 hours (30 days). `.env.example` and the
  account-deletion runbooks state the same value, and
  `TestLoadDeletionReversibleHoursDefault` guards it.
  The code wins; the example is wrong.
- The feed latency budget (S-5) is enforced by a timeout, not measured by a
  load test.
- Replay idempotency in ingestion (S-6) is an invariant of the schema and
  transaction, with no focused regression test.
- Backfill changes do not re-fetch history by design. A user who widens the
  window after a completed backfill sees no new old comments. This is
  documented in `docs/help/backfill-window.md` and is a product constraint,
  not a bug.
