## Context

LLM processing already counts tokens per batch (internal/llm metrics: request/token/duration counters in the `noumena/llm` meter) and billing already rolls up per-period comment usage (`billing_usage`, migration v12). What is missing is the bridge: token→cost conversion, durable storage, and exposure in `/api/billing/usage` + the dashboard. PRD LLM-07 acceptance explicitly requires per-comment cost in the billing dashboard — this closes that gap.

## Goals / Non-Goals

**Goals:**
- Record per-batch input/output token usage for every LLM call (classification + digest), attributed to user + channel + billing period.
- Derive cost cents from provider price cards (OpenCode primary/fallback + OpenAI/Anthropic paths if enabled) with explicit, versioned price tables.
- Persist a daily rollup keyed by (user, period, channel); expose `llm_cost_usd` on `GET /api/billing/usage` and month-to-date on `GET /api/billing/status`.
- Show cost in BillingView usage table + UsageBar; surface per-tenant ops metric `noumena_llm_cost_usd_total`.

**Non-Goals:**
- Re-pricing historical batches when provider prices change (prices snapshot at processing time).
- Cost allocation for shared-channel comments to specific trackers (ingested-once model: cost attributed to the user whose quota decremented, matching overage logic).
- Stripe invoicing from LLM cost (overage stays comment-count based); this is display/ops only.

## Decisions

1. **Price cards as code constants, versioned.** `internal/llm/prices.go` maps provider+model → {input, output} per-M tokens. Snapshot the price used into each rollup row (`price_version`) so retroactive price changes never rewrite history.
2. **Durable log: extend `billing_usage` with cost columns rather than a new table.** `billing_usage` already has (user, period, comments); add `llm_cost_usd` (daily rows merged at read time) — avoids a parallel rollup pipeline. Batch cost is added atomically in the same Tx2 that persists classification results (processor.go already persists results in a separate Tx2 after LLM calls).
3. **Digest costs included** (digest_worker writes its own usage rows) — a weekly digest can dominate a small account's spend; hiding it would mislead.
4. **Free tier shows cost too** (transparency) but overage/invoicing logic is untouched (Creator/Studio only, per AGENTS.md billing notes).
5. **API shape additive:** `usage` rows gain `llm_cost_usd`; `status` gains `llm_cost_mtd_usd`. Old clients ignore unknown fields.
6. **Frontend:** cost column in usage history table + MTD spend line in the plan card; UsageBar unchanged visually.

## Risks / Trade-offs

- **Price drift:** snapshot-per-batch means current pricing changes apply to new batches only — acceptable for display; ops monitors via `price_version` distribution.
- **Rounding:** cost stored in integer micro-cents (`cost_usd_micro`) to avoid float drift; converted at the API boundary.
- **Attribution dispute** (shared channels): documented rule — cost follows the user whose quota/period decremented; matches existing overage invoicing attribution.
- **Metric cardinality:** per-channel cost labels bounded by channel count; fine for OTel.
## Sync notes (fix-llm-duplicate-processing)

- The `fix-llm-duplicate-processing` change adds
  `noumena_llm_provider_calls_total{provider,retryable}` and
  `noumena_llm_job_attempts_total{provider,outcome}`. Use
  `job_attempts_total` (distinct job outcomes) as the cost denominator and
  `provider_calls_total` as the numerator when validating cost-per-comment:
  a ratio > 1.2 calls/job means duplicate work is inflating the bill.
- Per-comment attempts are now bounded (`LLM_MAX_ATTEMPTS`, default 3) and
  parked comments are excluded from claiming; cost rollups must not assume a
  failed comment will keep being billed.
- `comments.llm_attempts` / `comments.llm_next_retry_at` are available for
  per-comment retry accounting if cost attribution needs attempt counts.
