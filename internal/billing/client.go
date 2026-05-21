package billing

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/stripe/stripe-go/v76"
	billingportal "github.com/stripe/stripe-go/v76/billingportal/session"
	"github.com/stripe/stripe-go/v76/checkout/session"
	stripesub "github.com/stripe/stripe-go/v76/subscription"
	"github.com/stripe/stripe-go/v76/webhook"
)

// PriceMap maps (plan, interval) → Stripe price ID. Populated from config.
type PriceMap struct {
	CreatorMonthly string
	CreatorAnnual  string
	StudioMonthly  string
	StudioAnnual   string
	AgencyMonthly  string
	AgencyAnnual   string
}

// Lookup returns the Stripe price ID for a (plan, interval) pair. The empty
// string and an error are returned when the pair is unknown or the price is
// not configured.
func (m PriceMap) Lookup(plan, interval string) (string, error) {
	switch plan {
	case PlanCreator:
		switch interval {
		case IntervalMonth:
			return m.CreatorMonthly, nilIfSet(m.CreatorMonthly, "creator monthly")
		case IntervalYear:
			return m.CreatorAnnual, nilIfSet(m.CreatorAnnual, "creator annual")
		}
	case PlanStudio:
		switch interval {
		case IntervalMonth:
			return m.StudioMonthly, nilIfSet(m.StudioMonthly, "studio monthly")
		case IntervalYear:
			return m.StudioAnnual, nilIfSet(m.StudioAnnual, "studio annual")
		}
	case PlanAgency:
		switch interval {
		case IntervalMonth:
			return m.AgencyMonthly, nilIfSet(m.AgencyMonthly, "agency monthly")
		case IntervalYear:
			return m.AgencyAnnual, nilIfSet(m.AgencyAnnual, "agency annual")
		}
	}
	return "", fmt.Errorf("unknown plan/interval: %q/%q", plan, interval)
}

func nilIfSet(v, name string) error {
	if v == "" {
		return fmt.Errorf("price ID for %s is not configured", name)
	}
	return nil
}

// PlanFromPriceID is the inverse of PriceMap.Lookup: given a Stripe price id,
// return (plan, interval). Used by the customer.subscription.updated handler
// to map the authoritative Stripe state back into our local catalog.
func (m PriceMap) PlanFromPriceID(priceID string) (plan, interval string, ok bool) {
	switch priceID {
	case m.CreatorMonthly:
		return PlanCreator, IntervalMonth, true
	case m.CreatorAnnual:
		return PlanCreator, IntervalYear, true
	case m.StudioMonthly:
		return PlanStudio, IntervalMonth, true
	case m.StudioAnnual:
		return PlanStudio, IntervalYear, true
	case m.AgencyMonthly:
		return PlanAgency, IntervalMonth, true
	case m.AgencyAnnual:
		return PlanAgency, IntervalYear, true
	}
	return "", "", false
}

// Client is the narrow Stripe surface the billing handlers depend on. The
// real implementation talks to Stripe; tests use MockClient.
type Client interface {
	// CreateCheckoutSession creates a Stripe Checkout Session for the given
	// user and (plan, interval). The user_id is propagated through both
	// metadata and client_reference_id so webhooks can resolve the user
	// regardless of how the subscription is later modified.
	CreateCheckoutSession(ctx context.Context, userID, plan, interval, customerEmail string) (*stripe.CheckoutSession, error)

	// CreatePortalSession creates a Stripe Billing Portal session for an
	// existing customer.
	CreatePortalSession(ctx context.Context, customerID string) (*stripe.BillingPortalSession, error)

	// ConstructEvent verifies the Stripe signature on a webhook payload and
	// returns the parsed event.
	ConstructEvent(payload []byte, sig string) (stripe.Event, error)

	// GetSubscription fetches the authoritative Stripe-side state for a
	// subscription. Used by the subscription.updated handler.
	GetSubscription(ctx context.Context, subID string) (*stripe.Subscription, error)

	// CancelSubscription immediately cancels a subscription. Used by
	// the data-lifecycle deletion handler as a best-effort step before
	// queueing account deletion. See the StripeClient implementation
	// for rationale on the immediate-cancel choice.
	CancelSubscription(ctx context.Context, subID string) error

	// CancelSubscriptionAtPeriodEnd cancels a subscription at the end of
	// the current billing period. Used for annual plan cancellations where
	// the user retains access until the period ends.
	CancelSubscriptionAtPeriodEnd(ctx context.Context, subID string) error

	// PriceMap exposes the configured (plan, interval) → price-id mapping
	// so callers (including handlers and tests) can resolve plan from a
	// Stripe price-id.
	PriceMap() PriceMap
}

// StripeClient is the real implementation backed by stripe-go.
type StripeClient struct {
	webhookSecret string
	prices        PriceMap
	successURL    string
	cancelURL     string
}

// Config carries the construction parameters for a real StripeClient.
type Config struct {
	SecretKey         string
	WebhookSecret     string
	Prices            PriceMap
	SuccessURL        string
	CancelURL         string
}

// NewStripeClient configures the global stripe-go key and returns a Client.
// The legacy two-arg form is preserved as NewStripeClientLegacy for any code
// path that still constructs the old interface; the new code in cmd/api uses
// NewStripeClient(cfg).
func NewStripeClient(cfg Config) *StripeClient {
	stripe.Key = cfg.SecretKey
	return &StripeClient{
		webhookSecret: cfg.WebhookSecret,
		prices:        cfg.Prices,
		successURL:    cfg.SuccessURL,
		cancelURL:     cfg.CancelURL,
	}
}

func (c *StripeClient) PriceMap() PriceMap { return c.prices }

func (c *StripeClient) CreateCheckoutSession(ctx context.Context, userID, plan, interval, customerEmail string) (*stripe.CheckoutSession, error) {
	priceID, err := c.prices.Lookup(plan, interval)
	if err != nil {
		return nil, err
	}
	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(priceID), Quantity: stripe.Int64(1)},
		},
		SuccessURL:        stripe.String(c.successURL),
		CancelURL:         stripe.String(c.cancelURL),
		CustomerEmail:     stripe.String(customerEmail),
		ClientReferenceID: stripe.String(userID),
	}
	// Task 3.3: propagate user_id via metadata so subscription/invoice
	// events that lack a Checkout-session context can still resolve us.
	params.AddMetadata("user_id", userID)
	params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{}
	params.SubscriptionData.AddMetadata("user_id", userID)

	params.Context = ctx
	s, err := session.New(params)
	if err != nil {
		log.Printf("billing: stripe CheckoutSession.New failed: %v", err)
		return nil, err
	}
	return s, nil
}

func (c *StripeClient) CreatePortalSession(ctx context.Context, customerID string) (*stripe.BillingPortalSession, error) {
	if customerID == "" {
		return nil, errors.New("customerID is required")
	}
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(c.successURL),
	}
	params.Context = ctx
	s, err := billingportal.New(params)
	if err != nil {
		log.Printf("billing: stripe BillingPortalSession.New failed: %v", err)
		return nil, err
	}
	return s, nil
}

func (c *StripeClient) ConstructEvent(payload []byte, sig string) (stripe.Event, error) {
	return webhook.ConstructEvent(payload, sig, c.webhookSecret)
}

func (c *StripeClient) GetSubscription(ctx context.Context, subID string) (*stripe.Subscription, error) {
	params := &stripe.SubscriptionParams{}
	params.Context = ctx
	s, err := stripesub.Get(subID, params)
	return s, err
}

// CancelSubscription is the data-lifecycle hook: when a user requests
// account deletion, the API handler calls this on a best-effort basis
// before scheduling the deletion job. Failure here does NOT block the
// deletion (the user's intent is paramount); the caller logs the error
// and proceeds.
//
// We use the immediate-cancel form (not at_period_end=true) because:
//   1. The user has asked to delete the account; continuing to bill
//      them through the period would be hostile.
//   2. Stripe's webhook will still fire `customer.subscription.deleted`
//      which our existing webhook handler already processes — that's
//      the source of truth for the local subscription_status flip.
//
// Stripe errors with code `resource_missing` (the subscription is
// already canceled or never existed) are treated as success and
// returned as nil — there is nothing to cancel, so the caller's
// post-condition ("subscription is no longer billing the user") is
// already satisfied.
func (c *StripeClient) CancelSubscription(ctx context.Context, subID string) error {
	params := &stripe.SubscriptionCancelParams{}
	params.Context = ctx
	_, err := stripesub.Cancel(subID, params)
	if err != nil {
		var serr *stripe.Error
		if errors.As(err, &serr) && serr.Code == stripe.ErrorCodeResourceMissing {
			log.Printf("billing: CancelSubscription sub=%s already missing at Stripe (treating as success)", subID)
			return nil
		}
		return err
	}
	return nil
}

func (c *StripeClient) CancelSubscriptionAtPeriodEnd(ctx context.Context, subID string) error {
	params := &stripe.SubscriptionParams{
		CancelAtPeriodEnd: stripe.Bool(true),
	}
	params.Context = ctx
	_, err := stripesub.Update(subID, params)
	if err != nil {
		var serr *stripe.Error
		if errors.As(err, &serr) && serr.Code == stripe.ErrorCodeResourceMissing {
			log.Printf("billing: CancelSubscriptionAtPeriodEnd sub=%s already missing (treating as success)", subID)
			return nil
		}
		return err
	}
	return nil
}

// MockClient is a no-op Client used by tests. It returns deterministic
// values so tests can assert on them without hitting the Stripe API. The
// NextEvent / NextEventErr / Subscriptions fields let webhook tests prime
// the events that ConstructEvent / GetSubscription should return.
type MockClient struct {
	Prices PriceMap

	// NextEvent, if non-nil, is returned by ConstructEvent ignoring payload.
	NextEvent *stripe.Event
	// NextEventErr, if non-nil, is returned by ConstructEvent (used to
	// simulate signature-verification failures).
	NextEventErr error
	// Subscriptions allows GetSubscription to look up by id.
	Subscriptions map[string]*stripe.Subscription
	// Cancellations records the subscription IDs passed to
	// CancelSubscription so deletion-flow tests can assert behaviour.
	Cancellations []string
}

func NewMockClient() *MockClient { return &MockClient{} }

func (m *MockClient) PriceMap() PriceMap { return m.Prices }

func (m *MockClient) CreateCheckoutSession(_ context.Context, userID, plan, interval, _ string) (*stripe.CheckoutSession, error) {
	return &stripe.CheckoutSession{
		URL:               "https://checkout.stripe.com/pay/mock_session",
		ClientReferenceID: userID,
		Metadata:          map[string]string{"user_id": userID, "plan": plan, "interval": interval},
	}, nil
}

func (m *MockClient) CreatePortalSession(_ context.Context, customerID string) (*stripe.BillingPortalSession, error) {
	return &stripe.BillingPortalSession{
		URL:      "https://billing.stripe.com/p/mock_portal",
		Customer: customerID,
	}, nil
}

func (m *MockClient) ConstructEvent(_ []byte, _ string) (stripe.Event, error) {
	if m.NextEventErr != nil {
		return stripe.Event{}, m.NextEventErr
	}
	if m.NextEvent != nil {
		return *m.NextEvent, nil
	}
	return stripe.Event{}, nil
}

func (m *MockClient) GetSubscription(_ context.Context, subID string) (*stripe.Subscription, error) {
	if m.Subscriptions != nil {
		if s, ok := m.Subscriptions[subID]; ok {
			return s, nil
		}
	}
	return &stripe.Subscription{ID: subID, Status: stripe.SubscriptionStatusActive}, nil
}

// CancelSubscription on the mock simply records the id in a slice that
// tests can assert against. It never errors so deletion-flow tests can
// run end-to-end without simulating a Stripe outage; tests that DO
// want to simulate a failure should swap in a custom Client.
func (m *MockClient) CancelSubscription(_ context.Context, subID string) error {
	m.Cancellations = append(m.Cancellations, subID)
	return nil
}

func (m *MockClient) CancelSubscriptionAtPeriodEnd(_ context.Context, subID string) error {
	m.Cancellations = append(m.Cancellations, subID+":at_period_end")
	return nil
}
