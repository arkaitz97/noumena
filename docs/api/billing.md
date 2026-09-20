# Billing API Reference

> Source of truth: [`specs/07-billing.md`](../../specs/07-billing.md).
> Handler: [`internal/api/billing_handlers.go`](../../internal/api/billing_handlers.go).
> Plan catalog: [`internal/billing/plans.go`](../../internal/billing/plans.go).

The billing surface is built on Stripe Checkout and the Stripe
Customer Portal. Noumena never sees raw card data; all plan changes,
invoices, and refunds happen through Stripe's hosted UI, with state
reconciled into our database via webhooks.

## Plan limits

The catalog in
[`internal/billing/plans.go`](../../internal/billing/plans.go) is the
sole source of truth.

| Plan | Channels | Comments / month | Seats | Monthly | Annual |
|------|---------:|-----------------:|------:|--------:|-------:|
| `free`    | 1 | 1,000     | 1  | $0     | —      |
| `creator` | 3 | 50,000    | 1  | $29    | $278   |
| `studio`  | 15 | 250,000  | 5  | $99    | $950   |
| `agency`  | unlimited | 1,000,000 | 20 | $299 | $2,870 |

Overages: $0.50 per additional 1,000 comments on Creator and Studio.
Agency uses custom pricing handled outside this capability. Yearly
billing is a 20% discount baked into the price IDs.

`stripe_subscription_id` is persisted on the `users` row when the
checkout completes (column added by the
`backend-schema-and-env-fixes` change). It is used by
[`./user.md`](./user.md)'s `DELETE /api/user/account` flow to issue
a Stripe `Subscription.cancel` directly, so the user does not have to
visit the customer portal to stop billing on their way out.

## Endpoints

### POST /api/billing/checkout

- **Auth:** access JWT.
- **Request:**
  ```json
  { "plan": "creator", "interval": "month" }
  ```
  `plan` must be one of `creator`, `studio`, `agency` (free is not
  checkout-eligible). `interval` must be `month` or `year`.
- **Response:** `200 OK`,
  ```json
  { "checkout_url": "https://checkout.stripe.com/c/pay/cs_test_..." }
  ```
- **Errors:** `400 invalid plan` / `invalid interval` /
  `invalid request body`; `404 user not found`;
  `500 failed to create checkout session`.

### GET /api/billing/portal

- **Auth:** access JWT.
- **Response:** `200 OK`,
  `{ "portal_url": "https://billing.stripe.com/session/..." }`.
- **Errors:** `404 user not found`,
  `409 subscribe before opening the customer portal` (no
  `stripe_customer_id` on the user yet),
  `500 failed to create portal session`.

### GET /api/billing/status

- **Auth:** access JWT.
- **Response:** `200 OK`,
  ```json
  {
    "plan":               "creator",
    "status":             "active",
    "interval":           "month",
    "current_period_end": "2026-06-04T00:00:00Z",
    "channels_used":      2,
    "channels_limit":     3,
    "comments_used":      18234,
    "comments_limit":     50000,
    "overage_cost":       0.0,
    "llm_cost_mtd_usd":   1.234,
    "trial":              { "active": false, "ends_at": "", "days_remaining": 0 }
  }
  ```
  `status` is the local 4-state machine (`active`, `past_due`,
  `canceled`, `paused`). `interval` and `current_period_end` are
  omitted (`null`) when the user is on the free plan.
  `llm_cost_mtd_usd` is the month-to-date LLM processing cost for the
  current billing period (see `GET /api/billing/usage`).
- **Errors:** `404 user not found`, `500 internal error`.

### GET /api/billing/usage

- **Auth:** access JWT.
- **Query:** `periods` (optional, integer `1..12`, default `6`).
  Invalid values → `400 invalid periods`.
- **Response:** `200 OK`,
  ```json
  {
    "periods": [
      {
        "period_start": "2026-05-01T00:00:00Z",
        "period_end":   "2026-06-01T00:00:00Z",
        "comments_used": 18234,
        "quota":         50000,
        "llm_cost_usd":  1.2345
      }
    ]
  }
  ```
  Newest period first. `llm_cost_usd` is the accumulated LLM processing
  cost for that period (classification + weekly digests), stored
  internally in integer micro-USD and converted at this boundary. It is
  shown for **all plans including Free** (transparency); overage
  invoicing remains comment-count based. Costs follow the same
  attribution rule as overage: a shared channel's cost is billed to the
  primary tracker whose quota decremented. Prices are snapshotted per
  batch (`price_version`), so later price-card changes never rewrite
  historical rows.
- **Errors:** `400 invalid periods`; `500 internal error`.

### POST /webhooks/stripe (public)

The webhook receiver is mounted **outside** the auth middleware —
authentication is the `Stripe-Signature` header verified against
`STRIPE_WEBHOOK_SECRET`.

- **Body limit:** 1 MiB. Larger requests are rejected with
  `503 error reading request body`.
- **Idempotency:** every event id is recorded in
  `processed_stripe_events`. A redelivery short-circuits to
  `200 OK` with no side effects.
- **Handled events:**

  | Event | Effect |
  |---|---|
  | `checkout.session.completed` | Persist plan, interval, customer id, subscription id, period end. Reset usage counters. |
  | `invoice.payment_succeeded` | Mark subscription `active`, advance `current_period_end`, reset usage. |
  | `invoice.payment_failed` | Mark `past_due`, anchor `past_due_since` on first failure. Send `payment_failed` email (idempotent). |
  | `customer.subscription.created` / `.updated` | Map current price → plan/interval. Persist subscription id. Update status (`active` / `past_due` / `canceled` / `paused`). |
  | `customer.subscription.deleted` | Downgrade to `free`, clear `stripe_subscription_id`, pause non-included channels alphabetically. |
  | _any other_ | Logged at INFO and marked processed (Stripe stops retrying). |

- **Errors:** `400 invalid signature`; `500 internal error` for
  dispatch/handler failures (Stripe retries with backoff).

## Cancellation flow

1. User triggers cancellation. Two equivalent paths:
   - `DELETE /api/user/account` — see [`./user.md`](./user.md). Sets
     `users.deletion_requested_at`, calls
     `billing.CancelSubscription(stripe_subscription_id)`, schedules
     the data lifecycle worker, sends the `deletion_scheduled` email.
   - `GET /api/billing/portal` → user clicks "Cancel subscription" in
     Stripe's hosted portal. Stripe ultimately fires
     `customer.subscription.deleted`.
2. Stripe sends `customer.subscription.deleted`. Our webhook handler
   downgrades the user to `free`, clears `stripe_subscription_id`,
   and pauses any non-included channels (alphabetical) to fit the
   free-tier channel limit.
3. If the user requested account deletion, the lifecycle worker
   honours `DELETION_REVERSIBLE_HOURS` (default 720 = 30 days). See
   [`../ops/runbooks/account-deletion-and-gdpr.md`](../ops/runbooks/account-deletion-and-gdpr.md).

Errors from `Subscription.cancel` (other than `resource_missing`,
which the billing client treats as success) are **logged but do not
block** account deletion. The user's intent is paramount; the
eventual `customer.subscription.deleted` webhook will reconcile our
state regardless.

## Example

```sh
# Start a Creator monthly checkout
curl -i -X POST https://api.noumena/api/billing/checkout \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"plan":"creator","interval":"month"}'
```

## Related

- Configuration: see [`.env.example`](../../.env.example) for
  `BILLING_ENABLED`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, and
  every `STRIPE_PRICE_*` price ID.
- [`./user.md`](./user.md) for `DELETE /api/user/account`.
