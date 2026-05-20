# Spec: Billing & Subscriptions (Stripe)

## Overview
Integrate Stripe for subscription billing with Checkout, Customer Portal, webhook handling, metered overages, and plan enforcement. Support Free, Creator ($29), Studio ($99), and Agency ($299) tiers.

## Requirements
- BILL-01 to BILL-04: Tiered plans with channel and comment limits
- BILL-05: Stripe Checkout, no raw card data on our servers
- BILL-06: Webhook handler for Stripe events (idempotent)
- BILL-07: Trial expiry and payment failure emails with grace period
- BILL-08: Annual plans at 20% discount

## Plans Configuration

| Plan | Monthly Price | Annual Price | Channels | Comments/Month | Seats |
|------|--------------|--------------|----------|----------------|-------|
| free | $0 | — | 1 | 1,000 | 1 |
| creator | $29 | $278 | 3 | 50,000 | 1 |
| studio | $99 | $950 | 15 | 250,000 | 5 |
| agency | $299 | $2,870 | Unlimited | 1,000,000 | 20 |

Overages: $0.50 per additional 1,000 comments (Creator/Studio). Agency has custom pricing.

## Database Changes

Add to `users`:
- `trial_ends_at TIMESTAMPTZ` — for free trial tracking
- `subscription_status TEXT NOT NULL DEFAULT 'active'` — `active`, `past_due`, `canceled`, `paused`
- `current_period_end TIMESTAMPTZ`
- `billing_interval TEXT` — `month` | `year`

Create Stripe events table for idempotency:
```sql
CREATE TABLE processed_stripe_events (
    stripe_event_id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Create overage tracking:
```sql
CREATE TABLE comment_usage_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    count INT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## API Endpoints

### Authenticated Routes

#### POST /api/billing/checkout
- Body: `{ "plan": "creator" | "studio" | "agency", "interval": "month" | "year" }`
- Creates Stripe Checkout session
- Returns `{ "checkout_url": string }`
- User redirected to Stripe Checkout

#### GET /api/billing/portal
- Creates Stripe Customer Portal session
- Returns `{ "portal_url": string }`
- User manages subscription, payment methods, invoices

#### GET /api/billing/status
- Returns current subscription status:
```json
{
  "plan": "creator",
  "status": "active",
  "interval": "month",
  "current_period_end": "2026-04-13T00:00:00Z",
  "trial_ends_at": null,
  "channels_used": 2,
  "channels_limit": 3,
  "comments_used": 12400,
  "comments_limit": 50000,
  "overage_cost": 0
}
```

#### GET /api/billing/usage
- Returns detailed usage history
- Query params: `period` (current | previous)

### Webhook Routes (public, Stripe-signed)

#### POST /webhooks/stripe
- Verifies Stripe signature using webhook secret
- Processes events idempotently (check `processed_stripe_events` table)

**Events handled:**

1. `checkout.session.completed`:
   - Extract customer ID and subscription details
   - Update user: `subscription_plan`, `stripe_customer_id`, `comment_quota_monthly`, reset `comments_used_this_period`
   - Create Stripe customer if not exists

2. `invoice.payment_failed`:
   - Update `subscription_status = 'past_due'`
   - Send payment failure email
   - Start 3-day grace period
   - If not resolved in 3 days: `subscription_status = 'paused'`, stop LLM processing

3. `customer.subscription.deleted`:
   - Downgrade to free plan
   - Set `subscription_plan = 'free'`, `comment_quota_monthly = 1000`
   - If user has >1 channel active: pause excess channels, notify user to select active ones
   - `subscription_status = 'canceled'`

4. `invoice.payment_succeeded`:
   - Update `subscription_status = 'active'`
   - Reset `comments_used_this_period` to 0
   - Update `current_period_end`

5. `customer.subscription.updated`:
   - Handle plan changes (upgrades effective immediately, downgrades at period end)
   - Update `subscription_plan` and `comment_quota_monthly`

## Backend Implementation

### Stripe Client (`internal/billing/client.go`)
- Initialize with `stripe.Key = os.Getenv("STRIPE_SECRET_KEY")`
- Methods:
  - `CreateCheckoutSession(plan, interval, customerEmail) (*stripe.CheckoutSession, error)`
  - `CreatePortalSession(customerID) (*stripe.BillingPortalSession, error)`
  - `ConstructEvent(payload, sigHeader, webhookSecret) (stripe.Event, error)`

### Webhook Handler (`internal/api/billing_handlers.go`)
- Verify signature
- Idempotency check: `SELECT 1 FROM processed_stripe_events WHERE stripe_event_id = $1`
- Process event in a transaction
- Insert into `processed_stripe_events` on success
- Return 200 OK (even for duplicates)

### Quota Enforcement
- Check `comments_used_this_period >= comment_quota_monthly` before LLM processing
- If exceeded: pause processing, show upgrade banner
- On billing anniversary: reset `comments_used_this_period` to 0
- Daily cron job checks `quota_reset_at < NOW()` and resets counters

### Plan Change Logic
- **Upgrade**: effective immediately. Update quota, allow processing to resume.
- **Downgrade**: effective at period end (from Stripe). On `customer.subscription.updated` with `cancel_at_period_end` or plan downgrade:
  - Don't change plan immediately in DB
  - On `current_period_end` reached, apply downgrade
  - If channels > new limit: mark excess as `pending_removal`, notify user to select which to keep

### Trial Management
- Free tier is the default; no trial needed for paid plans unless we offer a free trial
- If offering 14-day trial: set `trial_ends_at = NOW() + 14 days` on first signup
- Email reminder at T-3 days before trial expiry
- On expiry without payment: downgrade to free

## Frontend Requirements

### Views
- `/settings/billing` (BillingView.vue):
  - Current plan card with usage bars (channels, comments)
  - Upgrade/downgrade buttons
  - Link to Stripe Customer Portal for payment management
  - Usage history chart
  - Overage warnings

### Upgrade Flow
1. User clicks "Upgrade" on billing page
2. Frontend calls `POST /api/billing/checkout` with selected plan
3. Redirects to Stripe Checkout URL
4. After payment, Stripe redirects to `/dashboard?checkout=success`
5. Frontend shows success banner, refreshes billing status

## Testing Checklist
- [ ] Checkout session creates correctly
- [ ] Webhook `checkout.session.completed` updates user plan and quota
- [ ] Duplicate webhooks are idempotent (200 OK, no DB change)
- [ ] Payment failure triggers grace period and email
- [ ] Subscription deletion downgrades to free and pauses excess channels
- [ ] Quota reset on billing anniversary
- [ ] Overage calculation is correct
- [ ] Plan downgrade defers to period end
- [ ] Annual discount applied correctly

## Files to Create/Modify
- `internal/billing/client.go` — implement Stripe methods
- `internal/api/billing_handlers.go` — extend with all endpoints
- `internal/api/billing_test.go` — webhook tests
- `internal/db/db.go` — add billing-related queries
- `frontend/src/views/settings/BillingView.vue` — billing UI

## Operations

### Environment Variables (task 11.1)

The following variables are read by `internal/config/config.go` at boot.
When `BILLING_ENABLED=true` the validator refuses to start unless every
required variable is set.

| Variable | Required when `BILLING_ENABLED=true` | Notes |
|----------|--------------------------------------|-------|
| `BILLING_ENABLED` | always | `true` / `false`. Default `false`. |
| `STRIPE_SECRET_KEY` | yes | `sk_test_...` or `sk_live_...`. |
| `STRIPE_WEBHOOK_SECRET` | yes | `whsec_...` from the Stripe dashboard webhook page. |
| `STRIPE_PRICE_CREATOR_MONTHLY` | yes | Stripe price ID for Creator monthly. |
| `STRIPE_PRICE_CREATOR_ANNUAL` | yes | |
| `STRIPE_PRICE_STUDIO_MONTHLY` | yes | |
| `STRIPE_PRICE_STUDIO_ANNUAL` | yes | |
| `STRIPE_PRICE_AGENCY_MONTHLY` | yes | |
| `STRIPE_PRICE_AGENCY_ANNUAL` | yes | |
| `BILLING_CHECKOUT_SUCCESS_URL` | yes | Absolute URL — typically `https://app.noumena.io/settings/billing/success`. |
| `BILLING_CHECKOUT_CANCEL_URL` | yes | Absolute URL — typically `https://app.noumena.io/settings/billing/cancel`. |

When `BILLING_ENABLED=false` (the rollback default), the missing variables
do NOT prevent startup; instead the API uses a mock billing client and the
`/webhooks/stripe` endpoint returns 200 with no side effects.

### Stripe Dashboard Setup (task 11.2)

Perform these steps once per environment (test mode for staging, live mode
for production). Save the resulting price IDs into the env vars above.

1. **Create products.** In *Products → Add product* create three products:
   `Creator`, `Studio`, `Agency`. Add a one-line description.
2. **Create prices.** For each product, add two recurring prices:
   - Monthly USD: $29 / $99 / $299, billing period = `monthly`.
   - Annual USD: $278 / $950 / $2,870, billing period = `yearly`.
   Copy each `price_…` ID into the matching `STRIPE_PRICE_*` env var.
3. **Configure the Customer Portal.** *Settings → Billing → Customer
   portal*. Enable "Allow customers to switch plans" with all six prices
   listed; allow "Cancel subscriptions"; set an appropriate "Business
   information" header.
4. **Create the webhook endpoint.** *Developers → Webhooks → Add
   endpoint*. URL = `https://api.noumena.io/webhooks/stripe`. Select
   events:
   - `checkout.session.completed`
   - `invoice.payment_succeeded`
   - `invoice.payment_failed`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   Reveal the **Signing secret** and copy it into `STRIPE_WEBHOOK_SECRET`.
5. **Configure billing portal return URL.** Use the same
   `BILLING_CHECKOUT_SUCCESS_URL` so users land back in the app after
   cancelling/updating.

### `BILLING_ENABLED` Rollout & Rollback (task 11.3)

`BILLING_ENABLED` is the master switch.

- **Rollout (off → on)**: set `BILLING_ENABLED=true` in the production
  environment, set every `STRIPE_*` and `BILLING_CHECKOUT_*` variable, and
  redeploy. The boot-time validator refuses to start if anything is
  missing, so a partial config can never reach traffic.
- **Rollback (on → off)**: flip `BILLING_ENABLED=false` and redeploy.
  Effects:
  - The mock billing client is used; `/api/billing/checkout`,
    `/api/billing/portal`, `/api/billing/status` continue to respond but
    do not talk to Stripe.
  - `/webhooks/stripe` returns 200 with no side effects (any in-flight
    Stripe deliveries during the redeploy window are absorbed without
    mutating users).
  - The daily quota cron (`StartQuotaCron`) keeps running; it has no
    Stripe dependency.
  - Existing user rows keep their `subscription_plan` /
    `subscription_status` values — no automatic downgrade.

### Runbook: User Stuck in `past_due` (task 11.4)

Symptom: a paying user reports their account is in `past_due` and they
believe they have paid.

1. Confirm in Stripe Dashboard → Customers that the most recent invoice
   for the customer is **paid**. If it is not paid, ask the user to
   retry the invoice via the Customer Portal — that path will trigger a
   fresh `invoice.payment_succeeded` webhook on completion.
2. If the invoice is paid in Stripe but our user row is still
   `past_due`:
   - Check `processed_stripe_events` for the event id of the relevant
     `invoice.payment_succeeded`. If present, the webhook arrived but
     either failed or was lost between Stripe and us — replay it from
     *Webhooks → Endpoints → (this endpoint) → Resend*.
   - If no audit row exists, our endpoint never received the delivery.
     Replay the event from the same Stripe panel.
3. If a replay fails because of a code-level handler bug, you can
   manually unstick the user with a single SQL transaction:
   ```sql
   UPDATE users
      SET subscription_status = 'active',
          past_due_since      = NULL,
          comments_used_this_period = 0,
          updated_at = NOW()
    WHERE id = '<user_uuid>';
   ```
   Always insert a row in `processed_stripe_events` referencing the
   manual recovery so a later replay is idempotent:
   ```sql
   INSERT INTO processed_stripe_events (stripe_event_id, event_type)
   VALUES ('manual-recovery-<ticket>', 'manual.recovery')
   ON CONFLICT DO NOTHING;
   ```
4. Document the incident: which event id was missing, root cause, and
   whether the daily `ExpirePastDueGrace` cron would have correctly
   transitioned the user to `paused` after the 3-day grace.

### Open blocker — live Stripe test E2E

End-to-end testing against a real Stripe test account (a checkout flow
that produces a real `checkout.session.completed` webhook delivery
verified by signature) is **not exercised in CI**. Setting it up
requires a live Stripe test-mode account with the price IDs above and a
publicly-reachable webhook receiver (e.g. via `stripe listen --forward-to`
in a developer environment). The Go-level handler is fully covered by
unit tests using `MockClient`; the live-fire test must be run manually
during release validation and is the documented blocker for tasks
6.1 / 6.2 / 6.3 going to production traffic.
