## Why

Doc-vs-code gap analysis found spec'd/PRD'd features that have zero implementation. These are user-visible product promises, not just hardening:

1. **Sentiment scoring** — `specs/05-insights-api.md:81` (`GET /api/insights/sentiment`), `PRD.md:110` DASH-02/LLM-03. No `sentiment_score` column, no prompt emission, no route, no frontend. A PRD Must with no code.
2. **Password change UI** — `specs/10-frontend-dashboard.md:162`. No account password section and no backend endpoint.
3. **Billing usage history** — `specs/07-billing.md:85` `GET /api/billing/usage`. No route; BillingView shows only a current UsageBar.
4. **Seat limits** — `specs/07-billing.md:16` Studio 5 / Agency 20. `internal/billing/plans.go:37` "v1 ignores this"; workspace invites unlimited.
5. **Trials never start** — `PRD.md:137` BILL-07. `trial_ends_at` is read (`billing.go:273` for status `trialing`) but never written; the trial-reminder cron is dead code; marketing promises a 14-day trial.
6. **LLM provider failover** — `PRD.md:108` LLM-08, `specs/04-llm-processing.md` spec'd OpenAI primary + Anthropic fallback. Only the OpenCode client exists; no failover.
7. **Token-expiry in-app banner** — `PRD.md:68` CM-06 says "banner shown on login"; only the email notifier path exists.
8. **Overage invoicing** — `internal/billing/quota.go:106-119` logs/reports overage but never invoices it; overage revenue is uncollected.

## What Changes

- **Sentiment scoring**: add `sentiment_score` (+ optional label) to comments via LLM prompt output, expose `GET /api/insights/sentiment` (per channel/range), render a sentiment trend in the dashboard (new `SentimentChart.vue`).
- **Password change**: backend `POST /api/auth/password` (verify current, new password policy) + AccountView section.
- **Usage history**: `GET /api/billing/usage` returning per-period usage, wired into BillingView.
- **Seat limits**: enforce active-member count against plan seats on invite/role-change; block when full; surface remaining seats in UI.
- **Trials**: set `trial_ends_at = now + 14d` on signup (free/new plans), enforce trial display + countdown, activate the trial-reminder cron; align marketing.
- **LLM provider failover**: add an OpenAI client (and stub/full Anthropic) behind the `LLMClient` interface with the sliding error-window failover already described in `openspec/project.md` (5 errors/60s).
- **Token-expiry banner**: on login + periodic check, show an in-app banner for channels with tokens expiring within `TOKEN_EXPIRY_WARNING_DAYS`.
- **Overage invoicing**: a billing job that issues Stripe invoice/balance charges for logged overage (creator/studio plans), matching quota.go reporting.

## Capabilities

### New Capabilities

- `sentiment-analysis`: Per-comment sentiment scoring surfaced as an aggregate trend.
- `billing-usage-history`: Per-period usage history endpoint + UI.
- `llm-provider-failover`: Primary/fallback LLM provider switching on an error window.
- `seat-enforcement`: Workspace seat limits enforced server-side.

### Modified Capabilities

- `billing`: Trials start and expire; overage is invoiced.
- `insights-api`: Sentiment endpoint added.
- `llm-processing`: Failover across providers.
- `frontend-dashboard`: Password change, trial countdown, token-expiry banner, usage history.

## Impact

- Schema: `comments.sentiment_score`, `billing usage` table (migration); `users.trial_ends_at` writes
- `internal/llm/` (client interface + OpenAI client + failover), `internal/api/insights_handlers.go`, `internal/api/auth_handlers.go`, `internal/api/billing_handlers.go`, `internal/api/workspace_handlers.go` (seats), `internal/billing/` (trials, overage invoicing)
- Frontend: `AccountView.vue` (password), `BillingView.vue` (usage + trial), new `SentimentChart.vue`, token-expiry banner, dashboard wiring
- Tests across each area
- Ops: `OPENAI_API_KEY` (and `ANTHROPIC_API_KEY`) env optional; documented
