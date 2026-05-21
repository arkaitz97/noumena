## ADDED Requirements

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

## MODIFIED Requirements

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
