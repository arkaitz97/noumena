# Proposal: LLM Cost Analytics

## Why

PRD requirement LLM-07's acceptance criterion — "per-comment cost is exposed in the billing dashboard" — is unimplemented: the usage view shows comment counts only (frontend/src/views/settings/BillingView.vue:205-262). Cost visibility is the #1 fear in creator SaaS economics: users cannot see what processing costs them, and we cannot validate unit economics or pricing without real per-period cost data. This closes that acceptance gap with data we already produce (token counts) — no new vendor.

## What Changes

- **Cost attribution:** LLM processor records per-batch token usage (input/output) and derives cost in cents from the active provider's price card (`internal/llm/` client + metrics already track tokens).
- **Persistence:** new `llm_cost_log` rows (user_id, period_start, channel_id, tokens, cost_cents) or cost columns on `billing_usage`; daily rollup by billing period.
- **API:** `GET /api/billing/usage` response gains per-period `llm_cost_usd` (+ optional per-channel breakdown); `GET /api/billing/status` gains month-to-date cost.
- **Frontend:** BillingView usage table adds a cost column + month-to-date total; UsageBar shows spend alongside quota.
- **Ops:** per-tenant cost metric `noumena_llm_cost_usd_total` for unit-economics monitoring; keep cost attribution consistent with overage invoicing (Creator/Studio only; Free/Agency excluded from per-comment charge display).

## Capabilities

### New Capabilities
- `llm-cost-analytics`: token→cost attribution, persistence, and rollup.

### Modified Capabilities
- `llm-processing`: record input/output token usage per batch and expose the price card used.
- `billing`: usage endpoint returns cost figures; status includes month-to-date spend.
- `frontend-dashboard`: cost display in usage table and quota bar.

## Impact

- `internal/llm/` — token accounting in processor + digest; provider price cards.
- `internal/db/schema.sql` + `migrations.go` — versioned migration for cost storage (ledger exists).
- `internal/api/billing_handlers.go` — usage/status response shape (additive, non-breaking).
- `frontend/src/views/settings/BillingView.vue`, `UsageBar.vue` — cost display.
- No Stripe changes, no new dependencies.