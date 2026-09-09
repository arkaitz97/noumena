## 1. Sentiment scoring

- [x] 1.1 Extend classification prompt to emit closed-set sentiment per comment
- [x] 1.2 Migration: add `comments.sentiment_score` (INTEGER -1/0/1)
- [x] 1.3 Persist sentiment in processor; backfill NULL rows idempotently
- [x] 1.4 `GET /api/insights/sentiment?channel_id=&days=` (clamp days {7,30,90})
- [x] 1.5 Frontend: SentimentChart.vue + wire into dashboard (and restore DEFERRED_TASKS-claimed files)
- [x] 1.6 Tests: prompt parse, persistence, endpoint, chart data mapping

## 2. Password change

- [x] 2.1 Backend `POST /api/auth/password` (verify current, policy, token_version bump)
- [x] 2.2 AccountView password section with inline validation
- [x] 2.3 Tests: wrong current rejected; success invalidates other sessions

## 3. Usage history

- [x] 3.1 `billing_usage` table (migration) + record per period
- [x] 3.2 `GET /api/billing/usage` (last N periods)
- [x] 3.3 BillingView table + keep UsageBar
- [x] 3.4 Tests: recording + endpoint

## 4. Seat limits

- [x] 4.1 Seats in plans.go (Free/Creator 1, Studio 5, Agency 20)
- [x] 4.2 Enforce on invite accept + role change (402/409 when full)
- [x] 4.3 "X of Y seats" in members UI
- [x] 4.4 Tests: cap enforced; over-cap workspaces logged not hard-blocked

## 5. Trials

- [x] 5.1 Set `trial_ends_at = now+14d` on new accounts
- [x] 5.2 Derive trial status; BillingView countdown
- [x] 5.3 Activate trial-reminder cron at configured offsets
- [x] 5.4 Align Landing/FAQ trial copy
- [x] 5.5 Tests: signup sets trial; countdown UI; reminder fires

## 6. LLM provider failover

- [x] 6.1 `LLMClient` interface + openaiClient (same contract)
- [x] 6.2 Optional anthropicClient
- [x] 6.3 `failingoverLLMClient` (5 errors/60s → permanent flip for process)
- [x] 6.4 Config LLM_PRIMARY/LLM_FALLBACK/keys; default OpenCode-only
- [x] 6.5 Tests: failover flips; back to primary never; config validation

## 7. Token-expiry banner

- [x] 7.1 `GET /api/user/token-expiry` (or reuse sync-status)
- [x] 7.2 Frontend banner on login/feed load for expiring tokens; links to reconnect
- [x] 7.3 Tests: expiring token → banner; none → hidden

## 8. Overage invoicing

- [x] 8.1 Read overage rows; group by user/period
- [x] 8.2 Stripe invoice/balance charge + idempotency key (user, period)
- [x] 8.3 Mark-invoiced guard; config toggle; free plan skipped
- [x] 8.4 Tests: invoice created once; double-run idempotent

## 9. Verify

- [x] 9.1 `go vet`, `go build`, `go test ./...` green
- [x] 9.2 `npm run build` + vitest green
- [ ] 9.3 Manual: sentiment trend renders; password change logs out other sessions; trial countdown; overage invoice in Stripe test mode
