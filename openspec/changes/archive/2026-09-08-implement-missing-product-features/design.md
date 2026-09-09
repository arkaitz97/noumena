## Context

These features are documented as shipped/spec'd but absent. They split into two classes: (a) backend capability + schema (sentiment, usage, seats, trials, failover, overage), (b) frontend surfaces (password change, trial countdown, token-expiry banner, sentiment chart, usage history). All are additive — no behavior changes to existing endpoints, minimizing regression risk.

## Goals / Non-Goals

**Goals:**
- Deliver each spec'd feature to a shippable state.
- Keep existing API shapes unchanged.
- Feature-gate heavy items (failover) so they can land independently.

**Non-Goals:**
- Not building a new LLM aggregation pipeline (sentiment reuses existing classification path).
- Not adding per-comment cost tracking here (roadmap-deferred).
- Not building admin UI for overage (invoice via Stripe dashboard).

## Decisions

### 1. Sentiment scoring

Extend the existing classification prompt (`internal/llm/prompts.go`) to emit `sentiment: negative|neutral|positive` (closed-set) per comment; persist `comments.sentiment_score` (TEXT or INTEGER -1/0/1 — choose INTEGER for aggregation). Migration adds column; backfill re-runs the LLM classification for rows with NULL sentiment (idempotent, respects the existing claim/dedupe). New route `GET /api/insights/sentiment?channel_id=&days=` returns per-day counts/scores; frontend `SentimentChart.vue` (restore the files DEFERRED_TASKS claimed existed).

| Option | Rejected because |
|---|---|
| Separate LLM pass for sentiment | Doubles cost; classification pass can emit both |
| TEXT score ("positive") | Aggregation over strings is awkward; INTEGER easier |

### 2. Password change

`POST /api/auth/password` with `{current_password, new_password}`: verify current via bcrypt, enforce existing policy, bump `token_version` (log out other sessions), no email required. Frontend: AccountView password section with the same inline validation as reset.

### 3. Usage history

New `billing_usage` table (user_id, period_start, period_end, comments_used, quota) or reuse existing usage/audit rows; `GET /api/billing/usage` returns last N periods. BillingView renders a small table + keeps the UsageBar for current.

### 4. Seat enforcement

`plans.go` seats: Free 1, Creator 1, Studio 5, Agency 20. Enforce at: workspace invite acceptance + role-change to member when workspace already at seat cap → 402/409 "seat limit reached, upgrade". Track active members per workspace (`workspace_members`). Surface "X of Y seats" in the members UI.

### 5. Trials

On user creation (new accounts, non-invite) set `trial_ends_at = now + 14d`. Trial status derived as `now < trial_ends_at` and no paid plan. BillingView shows countdown; quota cron already handles `trialing` status (after the CHECK-constraint fix in the SQLite change). Trial-reminder cron (`trial_reminder.go`) starts firing at configured offsets. Landing copy matches reality.

### 6. LLM provider failover

Keep `LLMClient` interface; add `openaiClient` (Chat Completions, same batching/prompt contract) and optional `anthropicClient`; a `failingoverLLMClient` wrapper counts consecutive/errored calls (5 errors in 60s window) then flips to the fallback for the process lifetime, exactly as `openspec/project.md` describes. Config: `LLM_PRIMARY=opencode|openai`, `LLM_FALLBACK=...`, keys from env. Default remains OpenCode-only when unset (backward compatible).

### 7. Token-expiry banner

Backend already computes expiring tokens (email notifier); add `GET /api/user/token-expiry` (or reuse sync-status). Frontend: on login + after feed load, if any channel's token expires within `TOKEN_EXPIRY_WARNING_DAYS`, show the banner (works with the reconnect flow from `fix-frontend-plumbing`).

### 8. Overage invoicing

New periodic job: read overage rows from quota.go's usage log, group by user/period, create a Stripe invoice line item (or customer balance charge) for overage units × plan unit price, then mark rows invoiced (idempotent). Free plan skips. Config toggle `BILLING_OVERAGE_INVOICING_ENABLED`.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Sentiment backfill costs LLM spend | Idempotent + respects claim/dedupe; runs only on NULL rows; rate-limited |
| Seat enforcement surprises existing workspaces | Count current members at deploy; over-cap workspaces flagged in logs, not hard-blocked until next invite |
| Trials + free quota interplay | Trial users keep free quota; only billing status changes |
| Failover adds new secrets/keys | Optional env; default OpenCode; document |
| Overage invoicing double-charges | Idempotency key = (user, period); mark-invoiced guard; manual review dashboard |
