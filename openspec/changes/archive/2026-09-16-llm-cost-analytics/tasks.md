## 1. Cost engine

- [x] 1.1 `internal/llm/prices.go`: versioned price cards (OpenCode + OpenAI/Anthropic paths), `PriceVersion` constant
- [x] 1.2 Migration: `llm_cost_usd_micro` columns on `billing_usage` (per-period rollup) + `price_version` column
- [x] 1.3 Processor + digest: capture input/output tokens per call, convert via price card, persist in same Tx2 as classification results
- [x] 1.4 Daily rollup job (or write-time accumulation) keyed by (user, period, channel)

## 2. API

- [x] 2.1 `GET /api/billing/usage`: per-period `llm_cost_usd` (micro→USD conversion at boundary)
- [x] 2.2 `GET /api/billing/status`: `llm_cost_mtd_usd`
- [x] 2.3 Ops metric `noumena_llm_cost_usd_total` (labels: user, channel, price_version)
- [x] 2.4 Handler tests: usage rows include cost, status includes MTD, absent rows → zero

## 3. Frontend

- [x] 3.1 BillingView: cost column in usage history table + MTD spend in plan card; graceful fallback when fields absent
- [x] 3.2 Component tests for cost rendering + fallback

## 4. Docs

- [x] 4.1 Update `docs/api/billing.md` (usage/status response fields)
- [x] 4.2 Note cost-attribution rule (follows quota-decrementing user) in docs + specs