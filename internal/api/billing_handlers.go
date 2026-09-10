package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v76"

	"noumena/internal/billing"
	"noumena/internal/db"
	"noumena/internal/email"
	"noumena/internal/telemetry"
)

// BillingHandler owns the four endpoints introduced by the `billing` change:
//
//	POST /api/billing/checkout
//	GET  /api/billing/portal
//	GET  /api/billing/status
//	POST /webhooks/stripe (public, signature-verified)
//
// All persistence is delegated to the billing-specific helpers on
// *db.SQLiteQuery (see internal/db/billing.go). Webhook handling
// type-asserts to *db.SQLiteQuery so it can begin a real sql transaction
// — the same pattern used by InsightsHandler. In mock-mode (tests) we fall
// back to the DBQuery interface and pass nil for the tx.
type BillingHandler struct {
	Queries       db.BillingStore
	BillingClient billing.Client

	// Pool is optional. When set (in production via cmd/api/main.go), webhook
	// handlers wrap their side effects + audit row insert in a single
	// transaction. When nil (tests, mock mode) handlers fall back to
	// non-transactional execution against the interface.
	Pool TxBeginner

	// EmailClient is optional; when set, payment-failure webhook (12.3)
	// emits TemplatePaymentFailed via the legacy interface (which the
	// pipeline adapter routes to the new email pipeline). Nil in mock mode
	// or when EMAIL_ENABLED is off.
	Pipeline *email.Pipeline
}

// TxBeginner is the minimal DB surface BillingHandler needs. *sql.DB
// satisfies it. Tests can leave it nil.
type TxBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// ----------------------------------------------------------------------------
// POST /api/billing/checkout
// ----------------------------------------------------------------------------

type checkoutRequest struct {
	Plan     string `json:"plan"`
	Interval string `json:"interval"`
}

type checkoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
}

func (h *BillingHandler) CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	plan := strings.ToLower(strings.TrimSpace(req.Plan))
	interval := strings.ToLower(strings.TrimSpace(req.Interval))

	// Spec: free is rejected with 400; only the three paid plans are valid.
	if plan != billing.PlanCreator && plan != billing.PlanStudio && plan != billing.PlanAgency {
		http.Error(w, "invalid plan", http.StatusBadRequest)
		return
	}
	if interval != billing.IntervalMonth && interval != billing.IntervalYear {
		http.Error(w, "invalid interval", http.StatusBadRequest)
		return
	}

	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	sess, err := h.BillingClient.CreateCheckoutSession(r.Context(), userID, plan, interval, user.Email)
	if err != nil {
		log.Printf("billing: CreateCheckoutSession user=%s plan=%s interval=%s err=%v",
			userID, plan, interval, err)
		http.Error(w, "failed to create checkout session", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, checkoutResponse{CheckoutURL: sess.URL})
}

// ----------------------------------------------------------------------------
// GET /api/billing/portal
// ----------------------------------------------------------------------------

type portalResponse struct {
	PortalURL string `json:"portal_url"`
}

func (h *BillingHandler) CreatePortalSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if user.StripeCustomerID == nil || *user.StripeCustomerID == "" {
		http.Error(w, "subscribe before opening the customer portal", http.StatusConflict)
		return
	}

	sess, err := h.BillingClient.CreatePortalSession(r.Context(), *user.StripeCustomerID)
	if err != nil {
		log.Printf("billing: CreatePortalSession user=%s err=%v", userID, err)
		http.Error(w, "failed to create portal session", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, portalResponse{PortalURL: sess.URL})
}

// ----------------------------------------------------------------------------
// GET /api/billing/status
// ----------------------------------------------------------------------------

type statusResponse struct {
	Plan              string     `json:"plan"`
	Status            string     `json:"status"`
	Interval          *string    `json:"interval,omitempty"`
	CurrentPeriodEnd  *time.Time `json:"current_period_end,omitempty"`
	ChannelsUsed      int        `json:"channels_used"`
	ChannelsLimit     int        `json:"channels_limit"`
	ChannelsUnlimited bool       `json:"channels_unlimited"`
	CommentsUsed      int        `json:"comments_used"`
	CommentsLimit     int        `json:"comments_limit"`
	OverageCost       float64    `json:"overage_cost"`
	// Trial (implement-missing-product-features 5.2): derived, additive.
	Trial trialInfo `json:"trial"`
}

// trialInfo is the derived trial state consumed by the billing store's
// trial countdown. Active when trial_ends_at is in the future AND the user
// is not on a paid plan (trial users keep the free quota; only the billing
// status changes).
type trialInfo struct {
	Active        bool   `json:"active"`
	EndsAt        string `json:"ends_at,omitempty"`
	DaysRemaining int    `json:"days_remaining"`
}

// trialDaysRemaining mirrors the reminder-cron's day math: full days until
// expiry, rounded up, floor 0.
func trialDaysRemaining(endsAt time.Time) int {
	d := int(time.Until(endsAt).Hours()/24) + 1
	if d < 0 {
		d = 0
	}
	return d
}

func (h *BillingHandler) Status(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireBillingUser(w, r)
	if !ok {
		return
	}
	plan := user.SubscriptionPlan
	if plan == "" {
		plan = billing.PlanFree
	}
	limits := billing.LimitsFor(plan)
	channelCount, ok := h.activeChannelCount(w, r, user.ID)
	if !ok {
		return
	}
	overage, ok := h.overageCost(w, r, user, plan)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, buildStatusResponse(user, plan, limits, channelCount, overage))
}

// requireBillingUser resolves the caller and loads their user row.
func (h *BillingHandler) requireBillingUser(w http.ResponseWriter, r *http.Request) (*db.User, bool) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	user, err := h.Queries.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return nil, false
	}
	return user, true
}

func (h *BillingHandler) activeChannelCount(w http.ResponseWriter, r *http.Request, userID string) (int, bool) {
	channelCount, err := h.Queries.GetActiveChannelCount(r.Context(), userID)
	if err != nil {
		log.Printf("billing: GetActiveChannelCount user=%s err=%v", userID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return 0, false
	}
	return channelCount, true
}

// overageCost is only meaningful for plans that track overage; the SQL is
// safe to call for any plan but we skip the round-trip otherwise.
func (h *BillingHandler) overageCost(w http.ResponseWriter, r *http.Request, user *db.User, plan string) (float64, bool) {
	if !billing.TracksOverage(plan) {
		return 0, true
	}
	periodStart, periodEnd := statusOverageWindow(user)
	overage, err := h.Queries.GetOverageCostForCurrentPeriod(r.Context(), user.ID, periodStart, periodEnd)
	if err != nil {
		log.Printf("billing: GetOverageCostForCurrentPeriod user=%s err=%v", user.ID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return 0, false
	}
	return overage, true
}

// statusOverageWindow anchors at current_period_end if known, otherwise the
// current calendar window. This is intentionally a best-effort window for
// read-side rollup; the authoritative boundaries are the ones written when
// each row was inserted.
func statusOverageWindow(user *db.User) (time.Time, time.Time) {
	periodEnd := time.Now().Add(31 * 24 * time.Hour)
	if user.CurrentPeriodEnd != nil {
		periodEnd = *user.CurrentPeriodEnd
	}
	return periodEnd.Add(-31 * 24 * time.Hour), periodEnd
}

// statusTrial resolves the trial banner. The trial is active while the
// deadline is in the future AND there is no paid subscription: upgrades
// exit the trial immediately.
func statusTrial(user *db.User, plan string) trialInfo {
	trial := trialInfo{Active: false}
	if user.TrialEndsAt == nil {
		return trial
	}
	trial.EndsAt = user.TrialEndsAt.UTC().Format(time.RFC3339)
	if time.Now().Before(*user.TrialEndsAt) && !billing.IsPaidPlan(plan) {
		trial.Active = true
		trial.DaysRemaining = trialDaysRemaining(*user.TrialEndsAt)
	}
	return trial
}

func buildStatusResponse(user *db.User, plan string, limits billing.PlanLimits, channelCount int, overage float64) statusResponse {
	resp := statusResponse{
		Plan:              plan,
		Status:            user.SubscriptionStatus,
		Interval:          user.BillingInterval,
		CurrentPeriodEnd:  user.CurrentPeriodEnd,
		ChannelsUsed:      channelCount,
		ChannelsLimit:     limits.ChannelLimit,
		ChannelsUnlimited: limits.ChannelsUnlimited,
		CommentsUsed:      user.CommentsUsedThisPeriod,
		CommentsLimit:     limits.CommentQuotaMonthly,
		OverageCost:       overage,
		Trial:             statusTrial(user, plan),
	}
	if resp.Status == "" {
		resp.Status = "active"
	}
	return resp
}

// ----------------------------------------------------------------------------
// GET /api/billing/usage (implement-missing-product-features 3.2)
// ----------------------------------------------------------------------------

// BillingUsageStore is the narrow read surface the usage endpoint needs.
// *db.SQLiteQuery satisfies it via GetBillingUsage.
type BillingUsageStore interface {
	GetBillingUsage(ctx context.Context, userID string, limit int) ([]db.BillingUsage, error)
}

type usagePeriod struct {
	PeriodStart  string `json:"period_start"`
	PeriodEnd    string `json:"period_end"`
	CommentsUsed int    `json:"comments_used"`
	Quota        int    `json:"quota"`
}

type usageResponse struct {
	Periods []usagePeriod `json:"periods"`
}

const (
	usageDefaultPeriods = 6
	usageMaxPeriods     = 12
)

// Usage returns the user's last N usage periods, newest first. Billing is
// per-user in this app, so access control is simply the authenticated user
// (same as Status). Query param `periods` clamps to [1,12]; invalid values
// are rejected with 400.
func (h *BillingHandler) Usage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	periods := usageDefaultPeriods
	if q := r.URL.Query().Get("periods"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > usageMaxPeriods {
			http.Error(w, "invalid periods", http.StatusBadRequest)
			return
		}
		periods = n
	}

	store, ok := h.Queries.(BillingUsageStore)
	if !ok {
		// Mock-mode queries that predate this endpoint have no history;
		// return an empty list rather than an internal error.
		writeJSON(w, http.StatusOK, usageResponse{Periods: []usagePeriod{}})
		return
	}

	rows, err := store.GetBillingUsage(r.Context(), userID, periods)
	if err != nil {
		log.Printf("billing: GetBillingUsage user=%s err=%v", userID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := usageResponse{Periods: make([]usagePeriod, 0, len(rows))}
	for _, u := range rows {
		resp.Periods = append(resp.Periods, usagePeriod{
			PeriodStart:  u.PeriodStart.UTC().Format(time.RFC3339),
			PeriodEnd:    u.PeriodEnd.UTC().Format(time.RFC3339),
			CommentsUsed: u.CommentsUsed,
			Quota:        u.Quota,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// ----------------------------------------------------------------------------
// POST /webhooks/stripe (public, signature-verified)
// ----------------------------------------------------------------------------

const stripeWebhookMaxBodyBytes = int64(1 << 20) // 1 MiB

func (h *BillingHandler) WebhookStripe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, stripeWebhookMaxBodyBytes)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		// otel-instrumentation: body read failure is unverified — we
		// don't yet know the event type. Record under empty event +
		// handler_error so we can spot upstream/network issues.
		billing.RecordStripeWebhook(r.Context(), "", telemetry.OutcomeHandlerError)
		http.Error(w, "error reading request body", http.StatusServiceUnavailable)
		return
	}

	sig := r.Header.Get("Stripe-Signature")
	event, err := h.BillingClient.ConstructEvent(payload, sig)
	if err != nil {
		// Spec: invalid signature → 400, no side effects.
		log.Printf("billing webhook: signature verification failed: %v", err)
		// otel-instrumentation: closed-enum signature_invalid outcome.
		// Event type is unknown at this point (body unparsed).
		billing.RecordStripeWebhook(r.Context(), "", telemetry.OutcomeSignatureInvalid)
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	// Idempotency short-circuit: if we've already processed this event id,
	// respond 200 immediately without touching anything else.
	if event.ID != "" {
		processed, err := h.Queries.IsStripeEventProcessed(r.Context(), event.ID)
		if err != nil {
			log.Printf("billing webhook: IsStripeEventProcessed err=%v", err)
			billing.RecordStripeWebhook(r.Context(), string(event.Type), telemetry.OutcomeHandlerError)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if processed {
			// otel-instrumentation: dedupe path is its own outcome so
			// dashboards can distinguish "Stripe is retrying us" from
			// "we successfully applied a fresh event".
			billing.RecordStripeWebhook(r.Context(), string(event.Type), telemetry.OutcomeIdempotentDedupe)
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	if err := h.dispatchEvent(r.Context(), event); err != nil {
		log.Printf("billing webhook: dispatch type=%s id=%s err=%v",
			event.Type, event.ID, err)
		billing.RecordStripeWebhook(r.Context(), string(event.Type), telemetry.OutcomeHandlerError)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// otel-instrumentation: success path. Note this includes the
	// "unhandled event type" branch in applyEvent — Stripe sees a 200
	// either way, so the webhook is "successful" from the contract's
	// perspective. The "ignored" outcome is reserved for cases where
	// we explicitly drop an event without recording it as processed
	// (none currently — kept in the enum for future use).
	billing.RecordStripeWebhook(r.Context(), string(event.Type), telemetry.OutcomeSuccess)
	w.WriteHeader(http.StatusOK)
}

// dispatchEvent runs the event-specific side effects + audit insert in a
// single transaction when a real pool is available. With no pool (tests)
// it falls back to non-transactional execution; the audit insert still
// guards against future double-application.
func (h *BillingHandler) dispatchEvent(ctx context.Context, event stripe.Event) error {
	if h.Pool == nil {
		return h.applyEvent(ctx, nil, event)
	}
	tx, err := h.Pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := h.applyEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

// applyEvent writes the side effects for a single event and the matching
// processed_stripe_events row. tx may be nil in mock mode.
func (h *BillingHandler) applyEvent(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	if err := h.applyEventType(ctx, tx, event); err != nil {
		return err
	}
	return h.markProcessed(ctx, tx, event)
}

// eventHandlers maps Stripe event types to their side-effect handlers; new
// event types register here instead of growing a switch statement.
func (h *BillingHandler) eventHandlers() map[stripe.EventType]func(context.Context, *sql.Tx, stripe.Event) error {
	return map[stripe.EventType]func(context.Context, *sql.Tx, stripe.Event) error{
		"checkout.session.completed":    h.handleCheckoutCompleted,
		"invoice.payment_succeeded":     h.handleInvoicePaymentSucceeded,
		"invoice.payment_failed":        h.handleInvoicePaymentFailed,
		"customer.subscription.deleted": h.handleSubscriptionDeleted,
		// Stripe fires created for upgrades initiated outside Checkout (e.g.
		// API-driven trials); persistence semantics match updated.
		"customer.subscription.created": h.handleSubscriptionUpdated,
		"customer.subscription.updated": h.handleSubscriptionUpdated,
	}
}

func (h *BillingHandler) applyEventType(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	handler, ok := h.eventHandlers()[event.Type]
	if !ok {
		// Spec 4.6: log unhandled event types at INFO and still mark them
		// processed so Stripe stops retrying.
		log.Printf("billing webhook: unhandled event type=%s id=%s (marking processed)", event.Type, event.ID)
		return nil
	}
	return handler(ctx, tx, event)
}

func (h *BillingHandler) markProcessed(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	if event.ID == "" {
		// Some test events have empty ids; nothing to write.
		return nil
	}
	return h.Queries.InsertProcessedStripeEvent(ctx, tx, event.ID, string(event.Type))
}

// ----------------------------------------------------------------------------
// Event handlers
// ----------------------------------------------------------------------------

// stripeCheckoutSession is a narrow subset of stripe.CheckoutSession we
// decode by hand — using stripe.CheckoutSession directly is fine for the
// real client but tests prime event payloads as JSON, and the SDK type
// requires the full nested object graph.
type stripeCheckoutSession struct {
	ClientReferenceID string            `json:"client_reference_id"`
	Customer          string            `json:"customer"`
	Subscription      string            `json:"subscription"`
	Metadata          map[string]string `json:"metadata"`
}

func (h *BillingHandler) handleCheckoutCompleted(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	sess, err := decodeCheckoutSession(event)
	if err != nil {
		return err
	}
	userID := checkoutUserID(sess)
	if userID == "" {
		log.Printf("billing webhook: checkout.session.completed missing user_id (event=%s)", event.ID)
		return nil
	}
	upd, err := h.checkoutSubscriptionUpdate(ctx, userID, sess)
	if err != nil {
		return err
	}
	return h.Queries.ApplySubscriptionUpdate(ctx, tx, upd)
}

func decodeCheckoutSession(event stripe.Event) (stripeCheckoutSession, error) {
	var sess stripeCheckoutSession
	err := json.Unmarshal(event.Data.Raw, &sess)
	return sess, err
}

// checkoutUserID prefers session metadata, falling back to the
// client_reference_id propagated at Checkout creation time.
func checkoutUserID(sess stripeCheckoutSession) string {
	if id := sess.Metadata["user_id"]; id != "" {
		return id
	}
	return sess.ClientReferenceID
}

// checkoutSubscriptionUpdate resolves the authoritative state from Stripe
// (so plan/interval/period_end come from the source of truth) and builds the
// local subscription row update.
func (h *BillingHandler) checkoutSubscriptionUpdate(ctx context.Context, userID string, sess stripeCheckoutSession) (db.SubscriptionUpdate, error) {
	plan, interval := sess.Metadata["plan"], sess.Metadata["interval"]
	sub, err := h.authoritativeSubscription(ctx, sess)
	if err != nil {
		return db.SubscriptionUpdate{}, err
	}
	if p, i, ok := priceOfItem(sub, h.BillingClient.PriceMap()); ok {
		plan, interval = p, i
	}
	upd := newActiveSubscriptionUpdate(userID, plan, interval)
	attachCheckoutRefs(&upd, sess, periodEndOf(sub))
	return upd, nil
}

// authoritativeSubscription fetches the Stripe-side subscription; one-time
// payment sessions (no subscription id) resolve to nil.
func (h *BillingHandler) authoritativeSubscription(ctx context.Context, sess stripeCheckoutSession) (*stripe.Subscription, error) {
	if sess.Subscription == "" {
		return nil, nil
	}
	return h.BillingClient.GetSubscription(ctx, sess.Subscription)
}

// newActiveSubscriptionUpdate is the standard "subscription now active" row:
// plan quota in, usage reset, past-due cleared.
func newActiveSubscriptionUpdate(userID, plan, interval string) db.SubscriptionUpdate {
	if plan == "" {
		plan = billing.PlanCreator // sensible default; metadata should always be present.
	}
	limits := billing.LimitsFor(plan)
	status, used := "active", 0
	return db.SubscriptionUpdate{
		UserID:                 userID,
		SubscriptionPlan:       &plan,
		BillingInterval:        &interval,
		CommentQuotaMonthly:    &limits.CommentQuotaMonthly,
		CommentsUsedThisPeriod: &used,
		SubscriptionStatus:     &status,
		PastDueSinceClear:      true,
	}
}

// attachCheckoutRefs persists the Stripe references carried by the session.
// An empty Subscription field (one-time payment sessions) leaves the existing
// column unchanged.
func attachCheckoutRefs(upd *db.SubscriptionUpdate, sess stripeCheckoutSession, periodEnd *time.Time) {
	if sess.Customer != "" {
		upd.StripeCustomerID = &sess.Customer
	}
	if sess.Subscription != "" {
		upd.StripeSubscriptionID = &sess.Subscription
	}
	upd.CurrentPeriodEnd = periodEnd
	upd.QuotaResetAt = periodEnd
}

type stripeInvoice struct {
	ID           string `json:"id"`
	Customer     string `json:"customer"`
	Subscription string `json:"subscription"`
	AmountDue    int64  `json:"amount_due"`
	Currency     string `json:"currency"`
	PeriodEnd    int64  `json:"period_end"`
	Lines        struct {
		Data []struct {
			Period struct {
				End int64 `json:"end"`
			} `json:"period"`
		} `json:"data"`
	} `json:"lines"`
}

func (h *BillingHandler) handleInvoicePaymentSucceeded(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	inv, err := decodeInvoice(event)
	if err != nil || inv.Customer == "" {
		return err
	}
	user, err := h.userByStripeCustomer(ctx, inv.Customer, "invoice.payment_succeeded")
	if err != nil || user == nil {
		return err
	}
	upd := paidInvoiceUpdate(user.ID, invoicePeriodEnd(inv))
	return h.Queries.ApplySubscriptionUpdate(ctx, tx, upd)
}

func decodeInvoice(event stripe.Event) (stripeInvoice, error) {
	var inv stripeInvoice
	err := json.Unmarshal(event.Data.Raw, &inv)
	return inv, err
}

// userByStripeCustomer resolves the local user for an invoice event, logging
// unknown customers under the given event name.
func (h *BillingHandler) userByStripeCustomer(ctx context.Context, customer, eventName string) (*db.User, error) {
	user, err := h.Queries.GetUserByStripeCustomerID(ctx, customer)
	if err != nil {
		return nil, err
	}
	if user == nil {
		log.Printf("billing webhook: %s for unknown customer=%s", eventName, customer)
	}
	return user, nil
}

// invoicePeriodEnd prefers the invoice line's period end (renewal anchor),
// falling back to invoice.period_end; nil leaves the column untouched.
func invoicePeriodEnd(inv stripeInvoice) *time.Time {
	if len(inv.Lines.Data) > 0 && inv.Lines.Data[0].Period.End != 0 {
		return unixPtr(inv.Lines.Data[0].Period.End)
	}
	return unixPtr(inv.PeriodEnd)
}

// paidInvoiceUpdate reactivates the subscription and resets usage for the
// new period.
func paidInvoiceUpdate(userID string, periodEnd *time.Time) db.SubscriptionUpdate {
	status, used := "active", 0
	upd := db.SubscriptionUpdate{
		UserID:                 userID,
		SubscriptionStatus:     &status,
		CommentsUsedThisPeriod: &used,
		PastDueSinceClear:      true,
	}
	upd.CurrentPeriodEnd = periodEnd
	upd.QuotaResetAt = periodEnd
	return upd
}

func (h *BillingHandler) handleInvoicePaymentFailed(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	inv, err := decodeInvoice(event)
	if err != nil || inv.Customer == "" {
		return err
	}
	user, err := h.Queries.GetUserByStripeCustomerID(ctx, inv.Customer)
	if err != nil || user == nil {
		return err
	}
	if err := h.Queries.ApplySubscriptionUpdate(ctx, tx, pastDueUpdate(user)); err != nil {
		return err
	}
	h.sendPaymentFailedEmail(ctx, user, inv)
	return nil
}

// pastDueUpdate marks the subscription past_due. past_due_since is only set
// on the *first* failure so the 3-day grace clock is anchored at the first
// miss; subsequent failures leave the existing timestamp in place.
func pastDueUpdate(user *db.User) db.SubscriptionUpdate {
	status := "past_due"
	upd := db.SubscriptionUpdate{UserID: user.ID, SubscriptionStatus: &status}
	if user.PastDueSince == nil {
		now := time.Now().UTC()
		upd.PastDueSince = &now
	}
	return upd
}

// sendPaymentFailedEmail 12.3: emits the payment_failed transactional email.
// Idempotency key includes the invoice ID so retried webhooks (Stripe
// redelivers on 5xx) collapse to one send. Pipeline+UI surface only — a
// failure to enqueue must not roll back the DB transition.
func (h *BillingHandler) sendPaymentFailedEmail(ctx context.Context, user *db.User, inv stripeInvoice) {
	if h.Pipeline == nil || inv.ID == "" {
		return
	}
	msg := email.Message{
		ToUserID:       user.ID,
		ToAddress:      user.Email,
		Template:       email.TemplatePaymentFailed,
		IdempotencyKey: "payment_failed:" + user.ID + ":" + inv.ID,
		Variables: map[string]any{
			"PlanName": "Creator", "Amount": float64(inv.AmountDue) / 100.0,
			"Currency": strings.ToUpper(inv.Currency), "PortalURL": "",
		},
	}
	if err := h.Pipeline.Send(ctx, msg); err != nil {
		log.Printf("payment_failed email enqueue failed user=%s invoice=%s: %v", user.ID, inv.ID, err)
	}
}

type stripeSubscriptionEvent struct {
	ID       string            `json:"id"`
	Customer string            `json:"customer"`
	Status   string            `json:"status"`
	Metadata map[string]string `json:"metadata"`
}

func (h *BillingHandler) handleSubscriptionUpdated(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	s, err := decodeSubscriptionEvent(event)
	if err != nil || s.ID == "" {
		return err
	}
	sub, err := h.BillingClient.GetSubscription(ctx, s.ID) // authoritative state per spec 5.4
	if err != nil || sub == nil {
		return err
	}
	user, err := h.resolveUserBySubEvent(ctx, s)
	if err != nil {
		return err
	}
	if user == nil {
		log.Printf("billing webhook: customer.subscription.updated unknown user (sub=%s)", s.ID)
		return nil
	}
	return h.applySubscriptionState(ctx, tx, user, s.ID, sub)
}

func decodeSubscriptionEvent(event stripe.Event) (stripeSubscriptionEvent, error) {
	var s stripeSubscriptionEvent
	err := json.Unmarshal(event.Data.Raw, &s)
	return s, err
}

// resolveUserBySubEvent finds the local user for a subscription event:
// metadata.user_id wins, then the Stripe customer id.
func (h *BillingHandler) resolveUserBySubEvent(ctx context.Context, s stripeSubscriptionEvent) (*db.User, error) {
	if id := s.Metadata["user_id"]; id != "" {
		user, err := h.Queries.GetUserByID(ctx, id)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
		if user != nil {
			return user, nil
		}
	}
	if s.Customer == "" {
		return nil, nil
	}
	return h.Queries.GetUserByStripeCustomerID(ctx, s.Customer)
}

// applySubscriptionState persists the mapped Stripe state, then enforces the
// new plan when the subscription is active.
func (h *BillingHandler) applySubscriptionState(ctx context.Context, tx *sql.Tx, user *db.User, subID string, sub *stripe.Subscription) error {
	plan, interval := "", ""
	if p, i, ok := priceOfItem(sub, h.BillingClient.PriceMap()); ok {
		plan, interval = p, i
	}
	status := mapStripeStatus(string(sub.Status))
	upd := subscriptionStateUpdate(user.ID, subID, status, plan, interval, sub.CurrentPeriodEnd)
	if err := h.Queries.ApplySubscriptionUpdate(ctx, tx, upd); err != nil {
		return err
	}
	h.enforceActivePlan(ctx, tx, user, plan, status)
	return nil
}

// subscriptionStateUpdate maps status/plan/interval/period onto the local
// row; empty plan or interval leave those columns unchanged.
func subscriptionStateUpdate(userID, subID, status, plan, interval string, periodEnd int64) db.SubscriptionUpdate {
	upd := db.SubscriptionUpdate{UserID: userID, SubscriptionStatus: &status, StripeSubscriptionID: &subID}
	applyPlanInterval(&upd, plan, interval)
	upd.CurrentPeriodEnd = unixPtr(periodEnd)
	upd.PastDueSinceClear = status == "active"
	return upd
}

func applyPlanInterval(upd *db.SubscriptionUpdate, plan, interval string) {
	if plan != "" {
		upd.SubscriptionPlan = &plan
		l := billing.LimitsFor(plan)
		upd.CommentQuotaMonthly = &l.CommentQuotaMonthly
	}
	if interval != "" {
		upd.BillingInterval = &interval
	}
}

// enforceActivePlan re-applies channel limits and notifies downgrades after
// an activation; plan-less or inactive events are ignored.
func (h *BillingHandler) enforceActivePlan(ctx context.Context, tx *sql.Tx, user *db.User, plan, status string) {
	if plan == "" || status != "active" {
		return
	}
	limit := billing.LimitsFor(plan)
	h.pauseExcessChannels(ctx, tx, user.ID, plan, limit)
	h.notifyPlanDowngrade(ctx, user, plan, limit)
}

func (h *BillingHandler) pauseExcessChannels(ctx context.Context, tx *sql.Tx, userID, plan string, limit billing.PlanLimits) {
	if limit.ChannelsUnlimited {
		return
	}
	if err := h.Queries.PauseChannelsAlphabetically(ctx, tx, userID, limit.ChannelLimit); err != nil {
		log.Printf("billing webhook: pause excess channels user=%s plan=%s: %v", userID, plan, err)
	}
}

// notifyPlanDowngrade 7.4: send the downgrade notification when the plan
// moved to a different tier.
func (h *BillingHandler) notifyPlanDowngrade(ctx context.Context, user *db.User, plan string, limit billing.PlanLimits) {
	oldPlan := user.SubscriptionPlan
	if oldPlan == "" {
		oldPlan = billing.PlanFree
	}
	if plan == oldPlan || h.Pipeline == nil {
		return
	}
	msg := email.Message{
		Template:       email.TemplatePlanDowngrade,
		ToUserID:       user.ID,
		ToAddress:      user.Email,
		IdempotencyKey: "plan_downgrade:" + user.ID + ":" + plan,
		Variables: map[string]any{
			"OldPlan": oldPlan, "NewPlan": plan,
			"ChannelLimit": limit.ChannelLimit, "PausedChannels": !limit.ChannelsUnlimited,
		},
	}
	_ = h.Pipeline.Send(ctx, msg)
}

func (h *BillingHandler) handleSubscriptionDeleted(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	s, err := decodeSubscriptionEvent(event)
	if err != nil {
		return err
	}
	user, err := h.resolveUserBySubEvent(ctx, s)
	if err != nil {
		return err
	}
	if user == nil {
		log.Printf("billing webhook: customer.subscription.deleted unknown user (sub=%s)", s.ID)
		return nil
	}
	if err := h.Queries.ApplySubscriptionUpdate(ctx, tx, freePlanDowngrade(user.ID)); err != nil {
		return err
	}
	// Free plan keeps 1 channel; pause everything else alphabetically.
	return h.Queries.PauseChannelsAlphabetically(ctx, tx, user.ID, billing.LimitsFor(billing.PlanFree).ChannelLimit)
}

// freePlanDowngrade is the spec 5.5 downgrade: free plan, canceled status,
// billing interval / past-due / subscription reference cleared.
func freePlanDowngrade(userID string) db.SubscriptionUpdate {
	freePlan, status := billing.PlanFree, "canceled"
	freeLimits := billing.LimitsFor(freePlan)
	return db.SubscriptionUpdate{
		UserID:                    userID,
		SubscriptionPlan:          &freePlan,
		CommentQuotaMonthly:       &freeLimits.CommentQuotaMonthly,
		SubscriptionStatus:        &status,
		BillingIntervalClear:      true,
		PastDueSinceClear:         true,
		StripeSubscriptionIDClear: true,
	}
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// unixPtr converts a Stripe epoch-seconds field to a *time.Time; 0 (unset)
// maps to nil so the column stays untouched.
func unixPtr(sec int64) *time.Time {
	if sec == 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

// periodEndOf is the authoritative current-period end of a Stripe
// subscription (nil when absent).
func periodEndOf(sub *stripe.Subscription) *time.Time {
	if sub == nil {
		return nil
	}
	return unixPtr(sub.CurrentPeriodEnd)
}

// priceOfItem maps the subscription's first item price ID back to the
// configured (plan, interval) catalog.
func priceOfItem(sub *stripe.Subscription, prices billing.PriceMap) (string, string, bool) {
	if sub == nil || sub.Items == nil || len(sub.Items.Data) == 0 || sub.Items.Data[0].Price == nil {
		return "", "", false
	}
	return prices.PlanFromPriceID(sub.Items.Data[0].Price.ID)
}

// priceMapPlanInterval inspects a Stripe Subscription's first item and
// returns the matching (plan, interval) from the configured PriceMap.
func (h *BillingHandler) priceMapPlanInterval(sub *stripe.Subscription) (string, string, bool) {
	return priceOfItem(sub, h.BillingClient.PriceMap())
}

// stripeStatusMap narrows the Stripe subscription status set onto the 4-value
// local state machine.
var stripeStatusMap = map[string]string{
	"active": "active", "trialing": "active",
	"past_due": "past_due", "unpaid": "past_due",
	"canceled": "canceled", "incomplete_expired": "canceled",
	"paused": "paused",
}

// mapStripeStatus narrows the Stripe subscription status set onto the
// 4-value local state machine.
func mapStripeStatus(s string) string {
	if mapped, ok := stripeStatusMap[s]; ok {
		return mapped
	}
	return "active"
}

// (writeJSON is defined in auth_handlers.go and reused here.)
