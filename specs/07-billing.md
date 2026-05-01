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
