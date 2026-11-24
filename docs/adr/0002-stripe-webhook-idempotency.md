# 0002. Make Stripe webhooks idempotent with a processed-event table inside the same transaction

## Status

Accepted. 2026-11-24.

## Context and Problem Statement

Stripe delivers each event at least once and may redeliver after a timeout.
Subscription state drives plan limits, quotas, channel pausing, and overage
invoicing. Applying the same event twice can double-count usage or flip a
plan twice.

The endpoint also receives unauthenticated traffic. It must reject a bad
signature before parsing, and it must bound the work a single request can
cause.

## Decision Drivers

- At-least-once delivery is the contract; the handler must be at-most-once in
  effect.
- Billing state must be recoverable from Stripe, not from local guesses.
- A duplicate delivery must be cheap and must return 200 so Stripe stops
  retrying.
- A concurrent duplicate must not apply side effects twice.

## Considered Options

1. **No dedupe.** Trust event order and Stripe retry timing.
2. **Dedupe with `processed_stripe_events`, side effects and the marker in one
   transaction, with a read short-circuit before dispatch.**
3. **Event sourcing.** Store every event and derive state in a projector.

## Decision Outcome

Chosen option 2.

`WebhookStripe` (`internal/api/billing_handlers.go`) caps the body at 1 MiB,
verifies `Stripe-Signature`, and, if the event has an ID, checks
`processed_stripe_events`. A known ID returns HTTP 200 without dispatch. When
a database pool is present, `dispatchEvent` opens a transaction, runs the
event handler, inserts the processed-event row, and commits. The table's
primary key is the Stripe event ID (`internal/db/schema.sql`).

Checkout carries `client_reference_id` = user ID, so `checkout.session.completed`
can attach the customer and subscription without a lookup by email. Where the
event includes a checkout session, the handler fetches the authoritative
subscription from Stripe before writing state. Unknown event types are logged
and marked processed so Stripe stops retrying.

Subscription state lives on `users`: `subscription_plan`,
`subscription_status`, `stripe_subscription_id`, `current_period_end`,
`billing_interval`, `past_due_since`. A partial unique index allows one user
per non-null subscription ID.

### Consequences

#### Good

- A replayed event is one indexed read and one 200 response.
- Side effects and the processed marker commit together, so a crash cannot
  leave state applied but unmarked.
- The partial unique index turns a duplicated subscription attachment into a
  database error instead of two billed users.

#### Bad

- The short-circuit read happens outside the transaction. Two concurrent
  deliveries of the same new event both pass it; the second transaction fails
  on the primary key and rolls back. That is correct only when a pool is
  present. In mock mode (`Pool == nil`) there is no transaction and the
  duplicate could re-apply side effects. This is a test-only mode.
- The table grows forever. There is no retention job for
  `processed_stripe_events`.
- An event handler that fails after a partial non-transactional side effect
  (for example an outbound email) can be retried by Stripe.

#### Neutral

- Unhandled event types are marked processed. If a future handler is added, old
  already-marked events will not be reprocessed.
- Email idempotency is separate: `payment_failed:<user>:<invoice>` keys the
  `email_send_log` row.

## Confirmation

- `go test ./internal/api/ -run 'TestStripeWebhook'`
- Specifically `TestStripeWebhookDuplicateShortCircuits` asserts HTTP 200 and
  zero subscription updates on a known event ID.
- CI job `fitness-webhook-idempotency`.

## More Information

- `internal/api/billing_handlers.go`
- `internal/db/billing_store.go`
- `internal/billing/plans.go`
- `docs/api/billing.md`
