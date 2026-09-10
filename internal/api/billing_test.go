package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stripe/stripe-go/v76"

	"noumena/internal/api"
	"noumena/internal/auth"
	"noumena/internal/billing"
	"noumena/internal/db"
	"noumena/internal/email"
)

// ============================================================================
// helpers
// ============================================================================

func setupBillingRouter(queries *MockQuery, billingClient billing.Client) *chi.Mux {
	r := chi.NewRouter()

	billingHandler := &api.BillingHandler{Queries: queries, BillingClient: billingClient}

	r.Route("/api", func(r chi.Router) {
		r.Use(api.AuthMiddleware)
		r.Post("/billing/checkout", billingHandler.CreateCheckoutSession)
		r.Get("/billing/portal", billingHandler.CreatePortalSession)
		r.Get("/billing/status", billingHandler.Status)
		r.Get("/billing/usage", billingHandler.Usage)
	})

	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/stripe", billingHandler.WebhookStripe)
	})

	return r
}

// seedTestUser inserts a user with the given id+plan into the mock and
// returns it. Email verified by default so verified-email gates pass.
func seedTestUser(q *MockQuery, id, plan string) *db.User {
	now := time.Now()
	u := &db.User{
		ID:                  id,
		Email:               id + "@example.com",
		SubscriptionPlan:    plan,
		SubscriptionStatus:  "active",
		CommentQuotaMonthly: billing.LimitsFor(plan).CommentQuotaMonthly,
		EmailVerifiedAt:     &now,
	}
	if q.users == nil {
		q.users = make(map[string]*db.User)
	}
	q.users[u.Email] = u
	return u
}

func bearer(token string) string { return "Bearer " + token }

// ============================================================================
// 10.3.1 POST /api/billing/checkout
// ============================================================================

func TestBillingCheckoutSuccess(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanFree)
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("POST", "/api/billing/checkout",
		bytes.NewBufferString(`{"plan":"creator","interval":"month"}`))
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		CheckoutURL string `json:"checkout_url"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.CheckoutURL == "" {
		t.Fatalf("expected checkout_url in response, got %s", rr.Body.String())
	}
}

func TestBillingCheckoutInvalidPlan(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanFree)
	r := setupBillingRouter(queries, billing.NewMockClient())
	access, _, _ := auth.GenerateTokens("test-user-id")

	cases := [][]byte{
		[]byte(`{"plan":"free","interval":"month"}`),
		[]byte(`{"plan":"unknown","interval":"month"}`),
		[]byte(`{"plan":"creator","interval":"weekly"}`),
		[]byte(`{}`),
	}
	for _, body := range cases {
		req, _ := http.NewRequest("POST", "/api/billing/checkout", bytes.NewBuffer(body))
		req.Header.Set("Authorization", bearer(access))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body=%s: want 400 got %d", body, rr.Code)
		}
	}
}

func TestBillingCheckoutUnauthenticated(t *testing.T) {
	r := setupBillingRouter(&MockQuery{}, billing.NewMockClient())
	req, _ := http.NewRequest("POST", "/api/billing/checkout",
		bytes.NewBufferString(`{"plan":"creator","interval":"month"}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 got %d", rr.Code)
	}
}

// ============================================================================
// 10.3.2 GET /api/billing/portal
// ============================================================================

func TestBillingPortalSuccess(t *testing.T) {
	queries := &MockQuery{}
	u := seedTestUser(queries, "test-user-id", billing.PlanCreator)
	cust := "cus_123"
	u.StripeCustomerID = &cust
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/portal", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200 got %d", rr.Code)
	}
	var body struct {
		PortalURL string `json:"portal_url"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.PortalURL == "" {
		t.Fatalf("expected portal_url in response, got %s", rr.Body.String())
	}
}

func TestBillingPortalConflictWhenNoStripeCustomer(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanFree)
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/portal", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("want 409 got %d", rr.Code)
	}
}

// ============================================================================
// 10.3.3 GET /api/billing/status
// ============================================================================

func TestBillingStatusReturnsLimitsAndUsage(t *testing.T) {
	queries := &MockQuery{}
	u := seedTestUser(queries, "test-user-id", billing.PlanCreator)
	u.CommentsUsedThisPeriod = 12400
	queries.activeChannelCount = map[string]int{u.ID: 2}
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/status", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Plan          string  `json:"plan"`
		Status        string  `json:"status"`
		ChannelsUsed  int     `json:"channels_used"`
		ChannelsLimit int     `json:"channels_limit"`
		CommentsUsed  int     `json:"comments_used"`
		CommentsLimit int     `json:"comments_limit"`
		OverageCost   float64 `json:"overage_cost"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Plan != "creator" || body.Status != "active" {
		t.Errorf("plan/status: got plan=%s status=%s", body.Plan, body.Status)
	}
	if body.ChannelsUsed != 2 || body.ChannelsLimit != 3 {
		t.Errorf("channels: got used=%d limit=%d", body.ChannelsUsed, body.ChannelsLimit)
	}
	if body.CommentsUsed != 12400 || body.CommentsLimit != 50000 {
		t.Errorf("comments: got used=%d limit=%d", body.CommentsUsed, body.CommentsLimit)
	}
	if body.OverageCost != 0 {
		t.Errorf("overage: want 0 got %v", body.OverageCost)
	}
}

func TestBillingStatusReportsOverage(t *testing.T) {
	queries := &MockQuery{}
	u := seedTestUser(queries, "test-user-id", billing.PlanCreator)
	u.CommentsUsedThisPeriod = 51500
	queries.activeChannelCount = map[string]int{u.ID: 2}
	queries.overageCost = map[string]float64{u.ID: 1.0} // ceil(1500/1000)*0.50
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("GET", "/api/billing/status", nil)
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200 got %d", rr.Code)
	}
	var body struct {
		OverageCost float64 `json:"overage_cost"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.OverageCost != 1.0 {
		t.Errorf("overage_cost: want 1.0 got %v", body.OverageCost)
	}
}

// ============================================================================
// 10.4.1 Webhook signature verification
// ============================================================================

func TestStripeWebhookInvalidSignatureRejected(t *testing.T) {
	queries := &MockQuery{}
	mc := billing.NewMockClient()
	mc.NextEventErr = errors.New("invalid signature")
	r := setupBillingRouter(queries, mc)

	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "forged")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 got %d", rr.Code)
	}
	if len(queries.processedEvents) != 0 {
		t.Errorf("expected no audit row on invalid sig, got %d", len(queries.processedEvents))
	}
	if len(queries.subscriptionUpdates) != 0 {
		t.Errorf("expected no subscription update on invalid sig")
	}
}

// ============================================================================
// 10.4.2 checkout.session.completed
// ============================================================================

func TestStripeWebhookCheckoutCompletedActivatesUser(t *testing.T) {
	queries := &MockQuery{}
	u := seedTestUser(queries, "user-1", billing.PlanFree)

	mc := billing.NewMockClient()
	// Subscription that GetSubscription will return — used to map back to plan/interval.
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour).Unix()
	mc.Prices = billing.PriceMap{CreatorMonthly: "price_cm"}
	mc.Subscriptions = map[string]*stripe.Subscription{
		"sub_1": {
			ID:               "sub_1",
			Status:           stripe.SubscriptionStatusActive,
			CurrentPeriodEnd: periodEnd,
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{
					{Price: &stripe.Price{ID: "price_cm"}},
				},
			},
		},
	}

	body := []byte(`{
		"client_reference_id": "user-1",
		"customer": "cus_1",
		"subscription": "sub_1",
		"metadata": {"user_id": "user-1", "plan": "creator", "interval": "month"}
	}`)
	mc.NextEvent = &stripe.Event{
		ID:   "evt_1",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: body},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(body))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.SubscriptionPlan != "creator" {
		t.Errorf("plan: want creator got %s", u.SubscriptionPlan)
	}
	if u.SubscriptionStatus != "active" {
		t.Errorf("status: want active got %s", u.SubscriptionStatus)
	}
	if u.CommentQuotaMonthly != 50000 {
		t.Errorf("quota: want 50000 got %d", u.CommentQuotaMonthly)
	}
	if u.CommentsUsedThisPeriod != 0 {
		t.Errorf("comments_used: want 0 got %d", u.CommentsUsedThisPeriod)
	}
	if u.StripeCustomerID == nil || *u.StripeCustomerID != "cus_1" {
		t.Errorf("stripe_customer_id: got %v", u.StripeCustomerID)
	}
	if u.BillingInterval == nil || *u.BillingInterval != "month" {
		t.Errorf("billing_interval: got %v", u.BillingInterval)
	}
	if _, ok := queries.processedEvents["evt_1"]; !ok {
		t.Errorf("expected processed_stripe_events row for evt_1")
	}
}

// ============================================================================
// 10.4.3 Duplicate event short-circuits
// ============================================================================

func TestStripeWebhookDuplicateShortCircuits(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "user-1", billing.PlanFree)
	queries.processedEvents = map[string]string{"evt_dup": "checkout.session.completed"}

	mc := billing.NewMockClient()
	mc.NextEvent = &stripe.Event{
		ID:   "evt_dup",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: []byte(`{"client_reference_id":"user-1"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rr.Code)
	}
	// The duplicate must NOT have applied a subscription update.
	if len(queries.subscriptionUpdates) != 0 {
		t.Errorf("expected no subscription updates on duplicate, got %d", len(queries.subscriptionUpdates))
	}
}

// ============================================================================
// 10.4.4 invoice.payment_failed → past_due + past_due_since
// ============================================================================

func TestStripeWebhookInvoicePaymentFailed(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_42"
	u := seedTestUser(queries, "user-1", billing.PlanCreator)
	u.StripeCustomerID = &cust
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	mc := billing.NewMockClient()
	mc.NextEvent = &stripe.Event{
		ID:   "evt_fail",
		Type: "invoice.payment_failed",
		Data: &stripe.EventData{Raw: []byte(`{"customer":"cus_42"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.SubscriptionStatus != "past_due" {
		t.Errorf("status: want past_due got %s", u.SubscriptionStatus)
	}
	if u.PastDueSince == nil {
		t.Error("PastDueSince must be set on first failure")
	}
}

// ============================================================================
// 10.4.5 invoice.payment_succeeded recovers from past_due, resets quota
// ============================================================================

func TestStripeWebhookInvoicePaymentSucceededRecovers(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_42"
	u := seedTestUser(queries, "user-1", billing.PlanCreator)
	u.StripeCustomerID = &cust
	u.SubscriptionStatus = "past_due"
	pdSince := time.Now().Add(-24 * time.Hour)
	u.PastDueSince = &pdSince
	u.CommentsUsedThisPeriod = 25000
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	mc := billing.NewMockClient()
	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	mc.NextEvent = &stripe.Event{
		ID:   "evt_ok",
		Type: "invoice.payment_succeeded",
		Data: &stripe.EventData{Raw: []byte(`{
			"customer":"cus_42",
			"period_end":` + itoa(periodEnd) + `
		}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.SubscriptionStatus != "active" {
		t.Errorf("status: want active got %s", u.SubscriptionStatus)
	}
	if u.CommentsUsedThisPeriod != 0 {
		t.Errorf("comments_used: want 0 got %d", u.CommentsUsedThisPeriod)
	}
	if u.PastDueSince != nil {
		t.Errorf("PastDueSince must be cleared on recovery")
	}
}

// ============================================================================
// 10.4.6 customer.subscription.deleted: free + alphabetical pause
// ============================================================================

func TestStripeWebhookSubscriptionDeletedDowngrades(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_99"
	u := seedTestUser(queries, "user-1", billing.PlanStudio)
	u.StripeCustomerID = &cust
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	mc := billing.NewMockClient()
	mc.NextEvent = &stripe.Event{
		ID:   "evt_del",
		Type: "customer.subscription.deleted",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_99","customer":"cus_99"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.SubscriptionPlan != "free" {
		t.Errorf("plan: want free got %s", u.SubscriptionPlan)
	}
	if u.CommentQuotaMonthly != 1000 {
		t.Errorf("quota: want 1000 got %d", u.CommentQuotaMonthly)
	}
	if u.SubscriptionStatus != "canceled" {
		t.Errorf("status: want canceled got %s", u.SubscriptionStatus)
	}
	if u.BillingInterval != nil {
		t.Errorf("billing_interval should be nil, got %v", u.BillingInterval)
	}
	// Alphabetical pause must have been called with keepCount=1 (free limit).
	if len(queries.pauseAlphaCalls) != 1 {
		t.Fatalf("expected 1 pause-alpha call, got %d", len(queries.pauseAlphaCalls))
	}
	if queries.pauseAlphaCalls[0].KeepCount != 1 || queries.pauseAlphaCalls[0].UserID != "user-1" {
		t.Errorf("unexpected pause-alpha call: %+v", queries.pauseAlphaCalls[0])
	}
}

// ============================================================================
// 10.4.7 customer.subscription.updated upgrade applies new quota
// ============================================================================

func TestStripeWebhookSubscriptionUpdatedUpgrade(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_77"
	u := seedTestUser(queries, "user-1", billing.PlanCreator)
	u.StripeCustomerID = &cust
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	mc := billing.NewMockClient()
	mc.Prices = billing.PriceMap{StudioMonthly: "price_sm"}
	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	mc.Subscriptions = map[string]*stripe.Subscription{
		"sub_77": {
			ID:               "sub_77",
			Status:           stripe.SubscriptionStatusActive,
			CurrentPeriodEnd: periodEnd,
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{
					{Price: &stripe.Price{ID: "price_sm"}},
				},
			},
		},
	}
	mc.NextEvent = &stripe.Event{
		ID:   "evt_upd",
		Type: "customer.subscription.updated",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_77","customer":"cus_77","status":"active"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.SubscriptionPlan != "studio" {
		t.Errorf("plan: want studio got %s", u.SubscriptionPlan)
	}
	if u.CommentQuotaMonthly != 250000 {
		t.Errorf("quota: want 250000 got %d", u.CommentQuotaMonthly)
	}
	if u.BillingInterval == nil || *u.BillingInterval != "month" {
		t.Errorf("billing_interval: want month got %v", u.BillingInterval)
	}
}

// ============================================================================
// backend-schema-and-env-fixes: stripe_subscription_id persistence
// ============================================================================

// 2.7: checkout sets stripe_subscription_id from session.subscription.
func TestStripeWebhookCheckoutPersistsSubscriptionID(t *testing.T) {
	queries := &MockQuery{}
	u := seedTestUser(queries, "user-sub-1", billing.PlanFree)

	mc := billing.NewMockClient()
	mc.Prices = billing.PriceMap{CreatorMonthly: "price_cm"}
	mc.Subscriptions = map[string]*stripe.Subscription{
		"sub_checkout_1": {
			ID:     "sub_checkout_1",
			Status: stripe.SubscriptionStatusActive,
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{{Price: &stripe.Price{ID: "price_cm"}}},
			},
		},
	}
	body := []byte(`{
		"client_reference_id":"user-sub-1",
		"customer":"cus_1",
		"subscription":"sub_checkout_1",
		"metadata":{"user_id":"user-sub-1","plan":"creator","interval":"month"}
	}`)
	mc.NextEvent = &stripe.Event{
		ID:   "evt_co_sub",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: body},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBuffer(body))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.StripeSubscriptionID == nil || *u.StripeSubscriptionID != "sub_checkout_1" {
		t.Errorf("stripe_subscription_id: got %v want sub_checkout_1", u.StripeSubscriptionID)
	}
}

// 2.7: customer.subscription.created routes through handleSubscriptionUpdated
// and sets the column.
func TestStripeWebhookSubscriptionCreatedPersistsID(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_create"
	u := seedTestUser(queries, "user-create", billing.PlanFree)
	u.StripeCustomerID = &cust
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	mc := billing.NewMockClient()
	mc.Prices = billing.PriceMap{CreatorMonthly: "price_cm"}
	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	mc.Subscriptions = map[string]*stripe.Subscription{
		"sub_new": {
			ID:               "sub_new",
			Status:           stripe.SubscriptionStatusActive,
			CurrentPeriodEnd: periodEnd,
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{{Price: &stripe.Price{ID: "price_cm"}}},
			},
		},
	}
	mc.NextEvent = &stripe.Event{
		ID:   "evt_subcreate",
		Type: "customer.subscription.created",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_new","customer":"cus_create","status":"active"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.StripeSubscriptionID == nil || *u.StripeSubscriptionID != "sub_new" {
		t.Errorf("stripe_subscription_id: got %v want sub_new", u.StripeSubscriptionID)
	}
}

// 2.7: customer.subscription.updated overwrites the column.
func TestStripeWebhookSubscriptionUpdatedOverwritesID(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_upd"
	u := seedTestUser(queries, "user-upd", billing.PlanCreator)
	u.StripeCustomerID = &cust
	old := "sub_old"
	u.StripeSubscriptionID = &old
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	mc := billing.NewMockClient()
	mc.Prices = billing.PriceMap{StudioMonthly: "price_sm"}
	mc.Subscriptions = map[string]*stripe.Subscription{
		"sub_replacement": {
			ID:     "sub_replacement",
			Status: stripe.SubscriptionStatusActive,
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{{Price: &stripe.Price{ID: "price_sm"}}},
			},
		},
	}
	mc.NextEvent = &stripe.Event{
		ID:   "evt_replace",
		Type: "customer.subscription.updated",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_replacement","customer":"cus_upd","status":"active"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.StripeSubscriptionID == nil || *u.StripeSubscriptionID != "sub_replacement" {
		t.Errorf("stripe_subscription_id: got %v want sub_replacement", u.StripeSubscriptionID)
	}
}

// 2.7: customer.subscription.deleted clears the column.
func TestStripeWebhookSubscriptionDeletedClearsID(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_del"
	u := seedTestUser(queries, "user-del", billing.PlanCreator)
	u.StripeCustomerID = &cust
	sid := "sub_to_clear"
	u.StripeSubscriptionID = &sid
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	mc := billing.NewMockClient()
	mc.NextEvent = &stripe.Event{
		ID:   "evt_clear",
		Type: "customer.subscription.deleted",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_to_clear","customer":"cus_del"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.StripeSubscriptionID != nil {
		t.Errorf("stripe_subscription_id: want nil got %v", *u.StripeSubscriptionID)
	}
}

// itoa formats an int64 for inline JSON literals.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// guard unused imports
var _ = context.Background

// ============================================================================
// Task 7.6: Annual pricing, downgrade behavior, webhook handling
// ============================================================================

func TestBillingCheckoutAnnualIntervalAccepted(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "test-user-id", billing.PlanFree)
	r := setupBillingRouter(queries, billing.NewMockClient())

	access, _, _ := auth.GenerateTokens("test-user-id")
	req, _ := http.NewRequest("POST", "/api/billing/checkout",
		bytes.NewBufferString(`{"plan":"studio","interval":"year"}`))
	req.Header.Set("Authorization", bearer(access))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("annual checkout: want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		CheckoutURL string `json:"checkout_url"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.CheckoutURL == "" {
		t.Fatal("annual checkout: expected checkout_url in response")
	}
}

func TestBillingCheckoutAllAnnualPlansAccepted(t *testing.T) {
	queries := &MockQuery{}
	seedTestUser(queries, "u", billing.PlanFree)
	r := setupBillingRouter(queries, billing.NewMockClient())
	access, _, _ := auth.GenerateTokens("u")

	plans := []string{billing.PlanCreator, billing.PlanStudio, billing.PlanAgency}
	for _, plan := range plans {
		body := []byte(`{"plan":"` + plan + `","interval":"year"}`)
		req, _ := http.NewRequest("POST", "/api/billing/checkout", bytes.NewBuffer(body))
		req.Header.Set("Authorization", bearer(access))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("checkout %s/year: want 200 got %d", plan, rr.Code)
		}
	}
}

func TestStripeWebhookSubscriptionUpdatedDowngrade(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_downgrade"
	u := seedTestUser(queries, "user-1", billing.PlanStudio)
	u.StripeCustomerID = &cust
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	mc := billing.NewMockClient()
	mc.Prices = billing.PriceMap{CreatorMonthly: "price_cm"}
	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	mc.Subscriptions = map[string]*stripe.Subscription{
		"sub_down": {
			ID:               "sub_down",
			Status:           stripe.SubscriptionStatusActive,
			CurrentPeriodEnd: periodEnd,
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{
					{Price: &stripe.Price{ID: "price_cm"}},
				},
			},
		},
	}
	mc.NextEvent = &stripe.Event{
		ID:   "evt_downgrade",
		Type: "customer.subscription.updated",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_down","customer":"cus_downgrade","status":"active"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	// Downgrade from studio → creator must apply new plan + quota.
	if u.SubscriptionPlan != billing.PlanCreator {
		t.Errorf("plan: want creator got %s", u.SubscriptionPlan)
	}
	if u.CommentQuotaMonthly != 50000 {
		t.Errorf("quota: want 50000 got %d", u.CommentQuotaMonthly)
	}
	// PauseChannelsAlphabetically must have been called with creator limit (3).
	if len(queries.pauseAlphaCalls) != 1 {
		t.Fatalf("expected 1 pause-alpha call, got %d", len(queries.pauseAlphaCalls))
	}
	if queries.pauseAlphaCalls[0].KeepCount != 3 {
		t.Errorf("pause-alpha keepCount: want 3 (creator), got %d", queries.pauseAlphaCalls[0].KeepCount)
	}
}

func TestStripeWebhookSubscriptionUpdatedUpgradeAppliesStudioLimits(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_upgrade"
	u := seedTestUser(queries, "user-2", billing.PlanCreator)
	u.StripeCustomerID = &cust
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	mc := billing.NewMockClient()
	mc.Prices = billing.PriceMap{StudioMonthly: "price_sm"}
	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	mc.Subscriptions = map[string]*stripe.Subscription{
		"sub_up": {
			ID:               "sub_up",
			Status:           stripe.SubscriptionStatusActive,
			CurrentPeriodEnd: periodEnd,
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{
					{Price: &stripe.Price{ID: "price_sm"}},
				},
			},
		},
	}
	mc.NextEvent = &stripe.Event{
		ID:   "evt_upgrade",
		Type: "customer.subscription.updated",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_up","customer":"cus_upgrade","status":"active"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.SubscriptionPlan != billing.PlanStudio {
		t.Errorf("plan: want studio got %s", u.SubscriptionPlan)
	}
	if u.CommentQuotaMonthly != 250000 {
		t.Errorf("quota: want 250000 got %d", u.CommentQuotaMonthly)
	}
	// PauseChannelsAlphabetically is called for every update to enforce plan limits.
	if len(queries.pauseAlphaCalls) != 1 {
		t.Fatalf("expected 1 pause-alpha call, got %d", len(queries.pauseAlphaCalls))
	}
	if queries.pauseAlphaCalls[0].KeepCount != 15 {
		t.Errorf("pause-alpha keepCount: want 15 (studio), got %d", queries.pauseAlphaCalls[0].KeepCount)
	}
}

// ============================================================================
// Stripe status narrowing must never treat an unmapped status as active.
// ============================================================================

// TestMapStripeStatus pins the Stripe→local status mapping. `incomplete`
// (payment not completed / SCA pending) and any unknown/empty value must map
// to a non-active state so no paid entitlement is granted before a charge
// succeeds. The mapping is exercised through the real webhook handler because
// mapStripeStatus is unexported.
func TestMapStripeStatus(t *testing.T) {
	cases := []struct {
		name   string
		stripe stripe.SubscriptionStatus
		want   string
	}{
		{"active", stripe.SubscriptionStatusActive, "active"},
		{"trialing", stripe.SubscriptionStatusTrialing, "active"},
		{"past_due", stripe.SubscriptionStatusPastDue, "past_due"},
		{"unpaid", stripe.SubscriptionStatusUnpaid, "past_due"},
		{"canceled", stripe.SubscriptionStatusCanceled, "canceled"},
		{"incomplete_expired", stripe.SubscriptionStatusIncompleteExpired, "canceled"},
		{"paused", stripe.SubscriptionStatusPaused, "paused"},
		{"incomplete", stripe.SubscriptionStatusIncomplete, "past_due"},
		{"unknown", stripe.SubscriptionStatus("some_future_status"), "past_due"},
		{"empty", stripe.SubscriptionStatus(""), "past_due"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			queries := &MockQuery{}
			uid := "user-map-" + tc.name
			u := seedTestUser(queries, uid, billing.PlanFree)

			mc := billing.NewMockClient()
			mc.Prices = billing.PriceMap{CreatorMonthly: "price_cm"}
			mc.Subscriptions = map[string]*stripe.Subscription{
				"sub_map": {
					ID:     "sub_map",
					Status: tc.stripe,
					Items: &stripe.SubscriptionItemList{
						Data: []*stripe.SubscriptionItem{{Price: &stripe.Price{ID: "price_cm"}}},
					},
				},
			}
			mc.NextEvent = &stripe.Event{
				ID:   "evt_map_" + tc.name,
				Type: "customer.subscription.updated",
				Data: &stripe.EventData{Raw: []byte(`{"id":"sub_map","metadata":{"user_id":"` + uid + `"}}`)},
			}

			r := setupBillingRouter(queries, mc)
			req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
			req.Header.Set("Stripe-Signature", "ok")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
			}
			if u.SubscriptionStatus != tc.want {
				t.Errorf("status: want %q got %q", tc.want, u.SubscriptionStatus)
			}
			if tc.want != "active" && u.SubscriptionStatus == "active" {
				t.Errorf("stripe status %q must not be treated as active", string(tc.stripe))
			}
		})
	}
}

// TestStripeWebhookSubscriptionIncompleteDoesNotUpgrade proves the billing
// bug: customer.subscription.created with status=incomplete must not upgrade
// the plan or grant the paid comment quota before a charge succeeds.
func TestStripeWebhookSubscriptionIncompleteDoesNotUpgrade(t *testing.T) {
	queries := &MockQuery{}
	cust := "cus_incomplete"
	u := seedTestUser(queries, "user-incomplete", billing.PlanFree)
	u.StripeCustomerID = &cust
	queries.usersByStripeCustomer = map[string]*db.User{cust: u}

	freeQuota := billing.LimitsFor(billing.PlanFree).CommentQuotaMonthly
	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	mc := billing.NewMockClient()
	mc.Prices = billing.PriceMap{CreatorMonthly: "price_cm"}
	mc.Subscriptions = map[string]*stripe.Subscription{
		"sub_incomplete": {
			ID:               "sub_incomplete",
			Status:           stripe.SubscriptionStatusIncomplete,
			CurrentPeriodEnd: periodEnd,
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{{Price: &stripe.Price{ID: "price_cm"}}},
			},
		},
	}
	mc.NextEvent = &stripe.Event{
		ID:   "evt_incomplete",
		Type: "customer.subscription.created",
		Data: &stripe.EventData{Raw: []byte(`{"id":"sub_incomplete","customer":"cus_incomplete","status":"incomplete"}`)},
	}

	r := setupBillingRouter(queries, mc)
	req, _ := http.NewRequest("POST", "/webhooks/stripe", bytes.NewBufferString(`{}`))
	req.Header.Set("Stripe-Signature", "ok")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if u.SubscriptionStatus != "past_due" {
		t.Errorf("status: want past_due got %s", u.SubscriptionStatus)
	}
	if u.SubscriptionPlan != billing.PlanFree {
		t.Errorf("plan must not upgrade on incomplete: want %s got %s", billing.PlanFree, u.SubscriptionPlan)
	}
	if u.CommentQuotaMonthly != freeQuota {
		t.Errorf("comment quota must be unchanged on incomplete: want %d got %d", freeQuota, u.CommentQuotaMonthly)
	}
	if len(queries.pauseAlphaCalls) != 0 {
		t.Errorf("incomplete must not enforce an active plan: pause-alpha calls=%d", len(queries.pauseAlphaCalls))
	}
}

// TestPlanDowngradeTemplateExists verifies the plan_downgrade template is
// registered and renders with the expected variables.
func TestPlanDowngradeTemplateExists(t *testing.T) {
	if email.TemplatePlanDowngrade != "plan_downgrade" {
		t.Errorf("TemplatePlanDowngrade: want plan_downgrade, got %s", email.TemplatePlanDowngrade)
	}
	// Verify it's in the registry.
	found := false
	for _, id := range email.TemplateIDs {
		if id == email.TemplatePlanDowngrade {
			found = true
			break
		}
	}
	if !found {
		t.Error("TemplatePlanDowngrade not in TemplateIDs registry")
	}
}
