## 1. Cost engine

- [ ] 1.1 `internal/llm/prices.go`: versioned price cards (OpenCode + OpenAI/Anthropic paths), `PriceVersion` constant
- [ ] 1.2 Migration: `llm_cost_usd_micro` columns on `billing_usage` (per-period rollup) + `price_version` column
- [ ] 1.3 Processor + digest: capture input/output tokens per call, convert via price card, persist in same Tx2 as classification results
- [ ] 1.4 Daily rollup job (or write-time accumulation) keyed by (user, period, channel)

## 2. API

- [ ] 2.1 `GET /api/billing/usage`: per-period `llm_cost_usd` (micro→USD conversion at boundary)
- [ ] 2.2 `GET /api/billing/status`: `llm_cost_mtd_usd`
- [ ] 2.3 Ops metric `noumena_llm_cost_usd_total` (labels: user, channel, price_version)
- [ ] 2.4 Handler tests: usage rows include cost, status includes MTD, absent rows → zero

## 3. Frontend

- [ ] 3.1 BillingView: cost column in usage history table + MTD spend in plan card; graceful fallback when fields absent
- [ ] 3.2 Component tests for cost rendering + fallback

## 4. Docs

- [ ] 4.1 Update `docs/api/billing.md` (usage/status response fields)
- [ ] 4.2 Note cost-attribution rule (follows quota-decrementing user) in docs + specs