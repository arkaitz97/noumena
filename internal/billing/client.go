package billing

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/stripe/stripe-go/v76"
	billingportal "github.com/stripe/stripe-go/v76/billingportal/session"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/customerbalancetransaction"
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

// priceEntry is one (plan, interval) → Stripe price ID row of the catalog.
type priceEntry struct {
	plan, interval, price string
}

// entries is the single source of truth for the price catalog: a data table
// instead of nested switch statements (open for new plans, closed for edits).
func (m PriceMap) entries() []priceEntry {
	return []priceEntry{
		{PlanCreator, IntervalMonth, m.CreatorMonthly},
		{PlanCreator, IntervalYear, m.CreatorAnnual},
		{PlanStudio, IntervalMonth, m.StudioMonthly},
		{PlanStudio, IntervalYear, m.StudioAnnual},
		{PlanAgency, IntervalMonth, m.AgencyMonthly},
		{PlanAgency, IntervalYear, m.AgencyAnnual},
	}
}

// Lookup returns the Stripe price ID for a (plan, interval) pair. The empty
// string and an error are returned when the pair is unknown or the price is
// not configured.
func (m PriceMap) Lookup(plan, interval string) (string, error) {
	for _, e := range m.entries() {
		if e.plan == plan && e.interval == interval {
			return e.price, nilIfSet(e.price, plan+" "+interval)
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
	for _, e := range m.entries() {
		if e.price != "" && e.price == priceID {
			return e.plan, e.interval, true
		}
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

	// CreateCustomerBalanceCharge applies a positive amount to the
	// customer's Stripe balance (they owe the overage; the debit lands on
	// their next finalized invoice). Used by the overage-invoicing job
	// (implement-missing-product-features 8.x). Callers pass an
	// idempotency key derived from (user, period) so Stripe collapses
	// retries into one transaction. Returns the balance transaction id.
	CreateCustomerBalanceCharge(ctx context.Context, customerID string, amountCents int64, description, idempotencyKey string, metadata map[string]string) (string, error)
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
	SecretKey     string
	WebhookSecret string
	Prices        PriceMap
	SuccessURL    string
	CancelURL     string
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

// CreateCustomerBalanceCharge posts a debit to the customer's balance via
// the /customers/{id}/balance_transactions API. amountCents must be
// positive (a negative value would be a credit). The Idempotency-Key
// header makes retries collapse server-side.
func (c *StripeClient) CreateCustomerBalanceCharge(ctx context.Context, customerID string, amountCents int64, description, idempotencyKey string, metadata map[string]string) (string, error) {
	if err := validateBalanceCharge(customerID, amountCents); err != nil {
		return "", err
	}
	params := balanceChargeParams(customerID, amountCents, description, idempotencyKey, metadata)
	params.Context = ctx
	return postBalanceCharge(customerID, amountCents, params)
}

// validateBalanceCharge guards the debit preconditions.
func validateBalanceCharge(customerID string, amountCents int64) error {
	if customerID == "" {
		return errors.New("customerID is required")
	}
	if amountCents <= 0 {
		return fmt.Errorf("amountCents must be positive, got %d", amountCents)
	}
	return nil
}

func balanceChargeParams(customerID string, amountCents int64, description, idempotencyKey string, metadata map[string]string) *stripe.CustomerBalanceTransactionParams {
	params := &stripe.CustomerBalanceTransactionParams{
		Customer:    stripe.String(customerID),
		Amount:      stripe.Int64(amountCents),
		Currency:    stripe.String(string(stripe.CurrencyUSD)),
		Description: stripe.String(description),
	}
	if idempotencyKey != "" {
		params.IdempotencyKey = stripe.String(idempotencyKey)
	}
	for k, v := range metadata {
		params.AddMetadata(k, v)
	}
	return params
}

func postBalanceCharge(customerID string, amountCents int64, params *stripe.CustomerBalanceTransactionParams) (string, error) {
	txn, err := customerbalancetransaction.New(params)
	if err != nil {
		log.Printf("billing: stripe balance transaction failed customer=%s amount=%d err=%v", customerID, amountCents, err)
		return "", err
	}
	return txn.ID, nil
}

func (c *StripeClient) CreateCheckoutSession(ctx context.Context, userID, plan, interval, customerEmail string) (*stripe.CheckoutSession, error) {
	priceID, err := c.prices.Lookup(plan, interval)
	if err != nil {
		return nil, err
	}
	params := c.checkoutParams(userID, priceID, customerEmail)
	params.Context = ctx
	return newCheckoutSession(params)
}

// checkoutParams builds the Checkout request. Task 3.3: propagate user_id via
// metadata so subscription/invoice events that lack a Checkout-session
// context can still resolve us.
func (c *StripeClient) checkoutParams(userID, priceID, customerEmail string) *stripe.CheckoutSessionParams {
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
	params.AddMetadata("user_id", userID)
	params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{}
	params.SubscriptionData.AddMetadata("user_id", userID)
	return params
}

func newCheckoutSession(params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
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
	params := &stripe.BillingPortalSessionParams{Customer: stripe.String(customerID), ReturnURL: stripe.String(c.successURL)}
	params.Context = ctx
	return newBillingPortalSession(params)
}

func newBillingPortalSession(params *stripe.BillingPortalSessionParams) (*stripe.BillingPortalSession, error) {
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
//  1. The user has asked to delete the account; continuing to bill
//     them through the period would be hostile.
//  2. Stripe's webhook will still fire `customer.subscription.deleted`
//     which our existing webhook handler already processes — that's
//     the source of truth for the local subscription_status flip.
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
	return ignoreMissingSubscription(subID, err)
}

func (c *StripeClient) CancelSubscriptionAtPeriodEnd(ctx context.Context, subID string) error {
	params := &stripe.SubscriptionParams{CancelAtPeriodEnd: stripe.Bool(true)}
	params.Context = ctx
	_, err := stripesub.Update(subID, params)
	return ignoreMissingSubscription(subID, err)
}

// ignoreMissingSubscription maps Stripe's `resource_missing` to success: the
// subscription is already canceled or never existed, so the caller's
// post-condition ("no longer billing the user") is already satisfied.
func ignoreMissingSubscription(subID string, err error) error {
	var serr *stripe.Error
	if errors.As(err, &serr) && serr.Code == stripe.ErrorCodeResourceMissing {
		log.Printf("billing: subscription %s already missing at Stripe (treating as success)", subID)
		return nil
	}
	return err
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
	// BalanceCharges records customer-balance debit attempts (overage
	// invoicing): customerID -> charges in call order.
	BalanceCharges map[string][]MockBalanceCharge
	// BalanceChargeErr, when non-nil, is returned by
	// CreateCustomerBalanceCharge.
	BalanceChargeErr error
}

// MockBalanceCharge is one recorded CreateCustomerBalanceCharge call.
type MockBalanceCharge struct {
	AmountCents    int64
	Description    string
	IdempotencyKey string
	Metadata       map[string]string
	ID             string
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

// CreateCustomerBalanceCharge on the mock records the charge and returns a
// deterministic transaction id. It never hits Stripe; tests assert against
// BalanceCharges.
func (m *MockClient) CreateCustomerBalanceCharge(_ context.Context, customerID string, amountCents int64, description, idempotencyKey string, metadata map[string]string) (string, error) {
	if m.BalanceChargeErr != nil {
		return "", m.BalanceChargeErr
	}
	if m.BalanceCharges == nil {
		m.BalanceCharges = map[string][]MockBalanceCharge{}
	}
	charges := m.BalanceCharges[customerID]
	id := fmt.Sprintf("cbtxn_%d", len(charges)+1)
	m.BalanceCharges[customerID] = append(charges, MockBalanceCharge{
		AmountCents:    amountCents,
		Description:    description,
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		ID:             id,
	})
	return id, nil
}
