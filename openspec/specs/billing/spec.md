# Billing Specification

## Purpose
Manages Stripe-backed subscription lifecycle: plan catalog, Checkout/Portal sessions, idempotent webhook processing, quota enforcement, and billing status reporting.

## Requirements

### Requirement: Plan Catalog
The system SHALL define four subscription plans with specific limits.

#### Scenario: Free Plan Limits
- **GIVEN** the plan catalog
- **WHEN** LimitsFor("free") is called
- **THEN** the system SHALL return: 1 channel, 1000 comments/month, 1 seat, $0 monthly, $0 annual

#### Scenario: Creator Plan Limits
- **GIVEN** the plan catalog
- **WHEN** LimitsFor("creator") is called
- **THEN** the system SHALL return: 3 channels, 50000 comments/month, 1 seat, $29 monthly, $278 annual

#### Scenario: Studio Plan Limits
- **GIVEN** the plan catalog
- **WHEN** LimitsFor("studio") is called
- **THEN** the system SHALL return: 15 channels, 250000 comments/month, 5 seats, $99 monthly, $950 annual

#### Scenario: Agency Plan Limits
- **GIVEN** the plan catalog
- **WHEN** LimitsFor("agency") is called
- **THEN** the system SHALL return: unlimited channels, 1000000 comments/month, 20 seats, $299 monthly, $2870 annual

#### Scenario: Unknown Plan Fallback
- **GIVEN** an unrecognized plan name
- **WHEN** LimitsFor is called
- **THEN** the system SHALL return the Free plan limits (defense in depth)

### Requirement: Checkout Session
The system SHALL create Stripe Checkout sessions for paid plan subscriptions.

#### Scenario: Create Checkout Session
- **GIVEN** an authenticated user
- **WHEN** the user calls POST /api/billing/checkout with a valid plan (creator/studio/agency) and interval (month/year)
- **THEN** the system SHALL create a Stripe Checkout session with user_id in metadata and return the checkout_url

#### Scenario: Reject Free Plan Checkout
- **GIVEN** an authenticated user
- **WHEN** the user calls POST /api/billing/checkout with plan=free
- **THEN** the system SHALL return 400 with "invalid plan"

#### Scenario: Invalid Interval
- **GIVEN** an authenticated user
- **WHEN** the user calls POST /api/billing/checkout with interval not in {month, year}
- **THEN** the system SHALL return 400 with "invalid interval"

### Requirement: Customer Portal
The system SHALL create Stripe Customer Portal sessions for subscription management.

#### Scenario: Create Portal Session
- **GIVEN** an authenticated user with a Stripe customer ID
- **WHEN** the user calls GET /api/billing/portal
- **THEN** the system SHALL create a Stripe Portal session and return the portal_url

#### Scenario: Portal Without Subscription
- **GIVEN** an authenticated user without a Stripe customer ID
- **WHEN** the user calls GET /api/billing/portal
- **THEN** the system SHALL return 409 with "subscribe before opening the customer portal"

### Requirement: Billing Status
The system SHALL provide the user's current billing state and usage.

#### Scenario: Get Billing Status
- **GIVEN** an authenticated user
- **WHEN** the user calls GET /api/billing/status
- **THEN** the system SHALL return plan, status, interval, current_period_end, channels_used, channels_limit, comments_used, comments_limit, and overage_cost

### Requirement: Stripe Webhook Processing
The system SHALL process Stripe webhook events idempotently within a single transaction, including subscription updates.

#### Scenario: checkout.session.completed
- **GIVEN** a valid Stripe webhook with event type checkout.session.completed
- **WHEN** the system receives and verifies the signature
- **THEN** the system SHALL update the user's subscription_plan, stripe_customer_id, stripe_subscription_id, billing_interval, comment_quota, subscription_status='active', current_period_end, and quota_reset_at

#### Scenario: invoice.payment_succeeded
- **GIVEN** a valid Stripe webhook with event type invoice.payment_succeeded
- **WHEN** the system processes it
- **THEN** the system SHALL set subscription_status='active', reset comments_used to 0, clear past_due_since, and update current_period_end + quota_reset_at

#### Scenario: invoice.payment_failed
- **GIVEN** a valid Stripe webhook with event type invoice.payment_failed
- **WHEN** the system processes it
- **THEN** the system SHALL set subscription_status='past_due', set past_due_since on first failure (anchor for 3-day grace clock), and enqueue a payment_failed email (idempotent by user_id + invoice_id)

#### Scenario: customer.subscription.updated
- **GIVEN** a valid Stripe webhook with event type customer.subscription.updated
- **WHEN** the system processes it
- **THEN** the system SHALL fetch authoritative state from Stripe, map the price ID to plan+interval, update subscription_status, plan, quota, clear past_due_since if status is 'active', and handle downgrade channel pausing if applicable

#### Scenario: customer.subscription.deleted
- **GIVEN** a valid Stripe webhook with event type customer.subscription.deleted
- **WHEN** the system processes it
- **THEN** the system SHALL downgrade the user to Free plan, set subscription_status='canceled', clear billing_interval and stripe_subscription_id, and pause channels alphabetically beyond the Free plan's 1-channel limit

#### Scenario: Duplicate Webhook Delivery
- **GIVEN** a Stripe event ID already recorded in processed_stripe_events
- **WHEN** the webhook handler receives it again
- **THEN** the system SHALL return 200 immediately without re-processing

#### Scenario: Invalid Webhook Signature
- **GIVEN** a webhook request with an invalid Stripe-Signature header
- **WHEN** the system attempts to construct the event
- **THEN** the system SHALL return 400 with no side effects

#### Scenario: Unknown Event Type
- **GIVEN** a valid Stripe webhook with an unhandled event type
- **WHEN** the system processes it
- **THEN** the system SHALL log the event at INFO, mark it as processed (so Stripe stops retrying), and return 200

### Requirement: Annual Plan Pricing
The system SHALL offer annual billing at a 20% discount compared to monthly pricing.

#### Scenario: Creator Annual Price
- **WHEN** LimitsFor("creator") is called with interval=year
- **THEN** the system SHALL return $278/year (equivalent to $23.17/month, 20% off $29/month)

#### Scenario: Studio Annual Price
- **WHEN** LimitsFor("studio") is called with interval=year
- **THEN** the system SHALL return $950/year (equivalent to $79.17/month, 20% off $99/month)

#### Scenario: Agency Annual Price
- **WHEN** LimitsFor("agency") is called with interval=year
- **THEN** the system SHALL return $2870/year (equivalent to $239.17/month, 20% off $299/month)

### Requirement: Subscription Downgrade Behavior
The system SHALL handle plan downgrades by pausing excess channels at the end of the billing period.

#### Scenario: Downgrade with Excess Channels
- **GIVEN** a user on Studio plan with 10 channels who downgrades to Creator (3 channel limit)
- **WHEN** the downgrade takes effect at period end
- **THEN** the system SHALL pause channels alphabetically beyond the 3-channel limit, send a notification listing which channels were paused, and prompt the user to select which 3 to keep active

#### Scenario: Downgrade Notification
- **GIVEN** a user who requested a downgrade
- **WHEN** the downgrade is scheduled
- **THEN** the system SHALL send an email explaining which channels will be paused and the effective date

### Requirement: Quota Cron
The system SHALL run daily billing maintenance to reset quotas and expire past-due grace periods.

#### Scenario: Reset Free Tier Quotas
- **GIVEN** free-tier users whose quota_reset_at is in the past
- **WHEN** the quota cron runs (default every 24h at 08:00 UTC)
- **THEN** the system SHALL reset comments_used_this_period to 0 and update quota_reset_at

#### Scenario: Expire Past-Due Grace
- **GIVEN** users in past_due status for more than PastDueGraceHours (default 72h / 3 days)
- **WHEN** the quota cron runs
- **THEN** the system SHALL transition them to 'paused' status and send a payment_grace_final email

### Requirement: Quota Middleware
The system SHALL block requests when a user has exceeded their comment processing quota.

#### Scenario: Quota Exceeded
- **GIVEN** a user whose comments_used_this_period >= comment_quota_monthly
- **WHEN** a request passes through QuotaMiddleware
- **THEN** the system SHALL return 402 with "quota exceeded, please upgrade your billing plan"

#### Scenario: Quota Available
- **GIVEN** a user whose comments_used_this_period < comment_quota_monthly
- **WHEN** a request passes through QuotaMiddleware
- **THEN** the system SHALL pass the request through to the next handler

### Requirement: Billing usage history
The system SHALL record per-period usage and expose it to the user.

#### Scenario: Usage history requested
- **GIVEN** an authenticated user with billing enabled
- **WHEN** `GET /api/billing/usage` is called
- **THEN** recent periods with comments used and quota are returned

### Requirement: Workspace seat limits
The system SHALL enforce plan seat limits on workspace membership.

#### Scenario: Invite at seat capacity
- **GIVEN** a workspace at its plan seat limit
- **WHEN** a new member is invited or role-changed to member
- **THEN** the action is rejected with a seat-limit error

### Requirement: Overage invoicing
The system SHALL invoice logged overage usage for billable plans.

#### Scenario: Overage accrues
- **GIVEN** a Creator/Studio account with overage in a period
- **WHEN** the overage-invoicing job runs
- **THEN** an invoice/charge is created exactly once per (user, period)

### Requirement: Trial lifecycle
The system SHALL assign trials to new accounts and surface trial state.

#### Scenario: New account signs up
- **GIVEN** a new account not created via invite
- **WHEN** the account is created
- **THEN** a trial period (14 days) is assigned

#### Scenario: Trial countdown
- **GIVEN** an account in its trial period
- **WHEN** the billing view is shown
- **THEN** the remaining trial days are displayed
