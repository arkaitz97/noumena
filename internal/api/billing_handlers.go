package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v76"

	"noumena/internal/billing"
	"noumena/internal/db"
	"noumena/internal/email"
	"noumena/internal/telemetry"
)

// BillingHandler owns the four endpoints introduced by the `billing` change:
//   POST /api/billing/checkout
//   GET  /api/billing/portal
//   GET  /api/billing/status
//   POST /webhooks/stripe (public, signature-verified)
//
// All persistence is delegated to the billing-specific helpers on
// *db.SQLiteQuery (see internal/db/billing.go). Webhook handling
// type-asserts to *db.SQLiteQuery so it can begin a real sql transaction
// — the same pattern used by InsightsHandler. In mock-mode (tests) we fall
// back to the DBQuery interface and pass nil for the tx.
type BillingHandler struct {
	Queries       db.DBQuery
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
	EmailClient email.Client

	// Pipeline is optional; when set (production w/ EMAIL_ENABLED=true),
	// webhook + grace-expiry handlers emit transactional templates that
	// have no legacy interface counterpart (payment_failed,
	// payment_grace_final, trial_expiry). Nil otherwise — sends are
	// skipped silently.
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
	Plan             string     `json:"plan"`
	Status           string     `json:"status"`
	Interval         *string    `json:"interval,omitempty"`
	CurrentPeriodEnd *time.Time `json:"current_period_end,omitempty"`
	ChannelsUsed     int        `json:"channels_used"`
	ChannelsLimit    int        `json:"channels_limit"`
	CommentsUsed     int        `json:"comments_used"`
	CommentsLimit    int        `json:"comments_limit"`
	OverageCost      float64    `json:"overage_cost"`
}

func (h *BillingHandler) Status(w http.ResponseWriter, r *http.Request) {
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

	plan := user.SubscriptionPlan
	if plan == "" {
		plan = billing.PlanFree
	}
	limits := billing.LimitsFor(plan)

	channelCount, err := h.Queries.GetActiveChannelCount(r.Context(), userID)
	if err != nil {
		log.Printf("billing: GetActiveChannelCount user=%s err=%v", userID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Overage: only meaningful for plans that track overage; the SQL is
	// safe to call for any plan but we skip the round-trip otherwise.
	var overage float64
	if billing.TracksOverage(plan) {
		// Period boundaries: anchor at current_period_end if known,
		// otherwise the current calendar window. This is intentionally a
		// best-effort window for read-side rollup; the authoritative
		// boundaries are the ones written when each row was inserted.
		periodEnd := time.Now().Add(31 * 24 * time.Hour)
		if user.CurrentPeriodEnd != nil {
			periodEnd = *user.CurrentPeriodEnd
		}
		periodStart := periodEnd.Add(-31 * 24 * time.Hour)
		overage, err = h.Queries.GetOverageCostForCurrentPeriod(r.Context(), userID, periodStart, periodEnd)
		if err != nil {
			log.Printf("billing: GetOverageCostForCurrentPeriod user=%s err=%v", userID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	resp := statusResponse{
		Plan:             plan,
		Status:           user.SubscriptionStatus,
		Interval:         user.BillingInterval,
		CurrentPeriodEnd: user.CurrentPeriodEnd,
		ChannelsUsed:     channelCount,
		ChannelsLimit:    limits.ChannelLimit,
		CommentsUsed:     user.CommentsUsedThisPeriod,
		CommentsLimit:    limits.CommentQuotaMonthly,
		OverageCost:      overage,
	}
	if resp.Status == "" {
		resp.Status = "active"
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
	switch event.Type {
	case "checkout.session.completed":
		if err := h.handleCheckoutCompleted(ctx, tx, event); err != nil {
			return err
		}
	case "invoice.payment_succeeded":
		if err := h.handleInvoicePaymentSucceeded(ctx, tx, event); err != nil {
			return err
		}
	case "invoice.payment_failed":
		if err := h.handleInvoicePaymentFailed(ctx, tx, event); err != nil {
			return err
		}
	case "customer.subscription.created":
		// Same persistence semantics as updated; Stripe fires this for
		// upgrades initiated outside Checkout (e.g. API-driven trials).
		if err := h.handleSubscriptionUpdated(ctx, tx, event); err != nil {
			return err
		}
	case "customer.subscription.updated":
		if err := h.handleSubscriptionUpdated(ctx, tx, event); err != nil {
			return err
		}
	case "customer.subscription.deleted":
		if err := h.handleSubscriptionDeleted(ctx, tx, event); err != nil {
			return err
		}
	default:
		// Spec 4.6: log unhandled event types at INFO and still mark them
		// processed so Stripe stops retrying.
		log.Printf("billing webhook: unhandled event type=%s id=%s (marking processed)",
			event.Type, event.ID)
	}

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
	var sess stripeCheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		return err
	}

	userID := sess.Metadata["user_id"]
	if userID == "" {
		userID = sess.ClientReferenceID
	}
	if userID == "" {
		log.Printf("billing webhook: checkout.session.completed missing user_id (event=%s)", event.ID)
		return nil
	}

	plan := sess.Metadata["plan"]
	interval := sess.Metadata["interval"]

	// If we have a subscription id, fetch the authoritative state from
	// Stripe so plan/interval/period_end come from the source of truth.
	var periodEnd *time.Time
	if sess.Subscription != "" {
		sub, err := h.BillingClient.GetSubscription(ctx, sess.Subscription)
		if err != nil {
			return err
		}
		if sub != nil {
			if p, i, ok := h.priceMapPlanInterval(sub); ok {
				plan = p
				interval = i
			}
			if sub.CurrentPeriodEnd != 0 {
				t := time.Unix(sub.CurrentPeriodEnd, 0).UTC()
				periodEnd = &t
			}
		}
	}

	if plan == "" {
		plan = billing.PlanCreator // sensible default; metadata should always be present.
	}

	limits := billing.LimitsFor(plan)
	status := "active"
	used := 0
	upd := db.SubscriptionUpdate{
		UserID:                 userID,
		SubscriptionPlan:       &plan,
		BillingInterval:        &interval,
		CommentQuotaMonthly:    &limits.CommentQuotaMonthly,
		CommentsUsedThisPeriod: &used,
		SubscriptionStatus:     &status,
		PastDueSinceClear:      true,
	}
	if sess.Customer != "" {
		c := sess.Customer
		upd.StripeCustomerID = &c
	}
	if sess.Subscription != "" {
		// Persist the subscription id so DeleteAccount can cancel it
		// later. Empty Subscription field (one-time payment sessions)
		// leaves the existing column unchanged.
		s := sess.Subscription
		upd.StripeSubscriptionID = &s
	}
	if periodEnd != nil {
		upd.CurrentPeriodEnd = periodEnd
		upd.QuotaResetAt = periodEnd
	}
	return h.Queries.ApplySubscriptionUpdate(ctx, tx, upd)
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
	var inv stripeInvoice
	if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
		return err
	}
	if inv.Customer == "" {
		return nil
	}
	user, err := h.Queries.GetUserByStripeCustomerID(ctx, inv.Customer)
	if err != nil {
		return err
	}
	if user == nil {
		log.Printf("billing webhook: invoice.payment_succeeded for unknown customer=%s", inv.Customer)
		return nil
	}

	// Period end: prefer the invoice line's period end (renewal anchor),
	// fall back to invoice.period_end. If neither is set, leave the column
	// untouched.
	var periodEnd *time.Time
	if len(inv.Lines.Data) > 0 && inv.Lines.Data[0].Period.End != 0 {
		t := time.Unix(inv.Lines.Data[0].Period.End, 0).UTC()
		periodEnd = &t
	} else if inv.PeriodEnd != 0 {
		t := time.Unix(inv.PeriodEnd, 0).UTC()
		periodEnd = &t
	}

	status := "active"
	used := 0
	upd := db.SubscriptionUpdate{
		UserID:                 user.ID,
		SubscriptionStatus:     &status,
		CommentsUsedThisPeriod: &used,
		PastDueSinceClear:      true,
	}
	if periodEnd != nil {
		upd.CurrentPeriodEnd = periodEnd
		upd.QuotaResetAt = periodEnd
	}
	return h.Queries.ApplySubscriptionUpdate(ctx, tx, upd)
}

func (h *BillingHandler) handleInvoicePaymentFailed(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	var inv stripeInvoice
	if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
		return err
	}
	if inv.Customer == "" {
		return nil
	}
	user, err := h.Queries.GetUserByStripeCustomerID(ctx, inv.Customer)
	if err != nil {
		return err
	}
	if user == nil {
		return nil
	}
	status := "past_due"
	now := time.Now().UTC()
	upd := db.SubscriptionUpdate{
		UserID:             user.ID,
		SubscriptionStatus: &status,
	}
	// Only set past_due_since on the *first* failure so the 3-day grace
	// clock is anchored at the first miss. Subsequent failures leave the
	// existing timestamp in place.
	if user.PastDueSince == nil {
		upd.PastDueSince = &now
	}
	if err := h.Queries.ApplySubscriptionUpdate(ctx, tx, upd); err != nil {
		return err
	}

	// 12.3: emit payment_failed transactional email. Idempotency key
	// includes the invoice ID so retried webhooks (Stripe redelivers on
	// 5xx) collapse to one send. Pipeline+UI surface only — failure to
	// enqueue must not roll back the DB transition.
	if h.Pipeline != nil && inv.ID != "" {
		key := "payment_failed:" + user.ID + ":" + inv.ID
		amount := float64(inv.AmountDue) / 100.0
		msg := email.Message{
			ToUserID:       user.ID,
			ToAddress:      user.Email,
			Template:       email.TemplatePaymentFailed,
			IdempotencyKey: key,
			Variables: map[string]any{
				"PlanName":  "Creator",
				"Amount":    amount,
				"Currency":  strings.ToUpper(inv.Currency),
				"PortalURL": "",
			},
		}
		if err := h.Pipeline.Send(ctx, msg); err != nil {
			log.Printf("payment_failed email enqueue failed user=%s invoice=%s: %v",
				user.ID, inv.ID, err)
		}
	}
	return nil
}

type stripeSubscriptionEvent struct {
	ID         string            `json:"id"`
	Customer   string            `json:"customer"`
	Status     string            `json:"status"`
	Metadata   map[string]string `json:"metadata"`
}

func (h *BillingHandler) handleSubscriptionUpdated(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	var s stripeSubscriptionEvent
	if err := json.Unmarshal(event.Data.Raw, &s); err != nil {
		return err
	}
	if s.ID == "" {
		return nil
	}

	// Authoritative state per spec 5.4.
	sub, err := h.BillingClient.GetSubscription(ctx, s.ID)
	if err != nil {
		return err
	}
	if sub == nil {
		return nil
	}

	// Resolve user: metadata.user_id wins, then by customer id.
	var user *db.User
	if id := s.Metadata["user_id"]; id != "" {
		user, err = h.Queries.GetUserByID(ctx, id)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	if user == nil && s.Customer != "" {
		user, err = h.Queries.GetUserByStripeCustomerID(ctx, s.Customer)
		if err != nil {
			return err
		}
	}
	if user == nil {
		log.Printf("billing webhook: customer.subscription.updated unknown user (sub=%s)", s.ID)
		return nil
	}

	// Map current price ID back to plan + interval.
	var plan, interval string
	if sub.Items != nil && len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
		if p, i, ok := h.BillingClient.PriceMap().PlanFromPriceID(sub.Items.Data[0].Price.ID); ok {
			plan = p
			interval = i
		}
	}

	status := mapStripeStatus(string(sub.Status))
	upd := db.SubscriptionUpdate{
		UserID:               user.ID,
		SubscriptionStatus:   &status,
		StripeSubscriptionID: &s.ID,
	}
	if plan != "" {
		upd.SubscriptionPlan = &plan
		l := billing.LimitsFor(plan)
		upd.CommentQuotaMonthly = &l.CommentQuotaMonthly
	}
	if interval != "" {
		upd.BillingInterval = &interval
	}
	if sub.CurrentPeriodEnd != 0 {
		t := time.Unix(sub.CurrentPeriodEnd, 0).UTC()
		upd.CurrentPeriodEnd = &t
	}
	if status == "active" {
		upd.PastDueSinceClear = true
	}
	if err := h.Queries.ApplySubscriptionUpdate(ctx, tx, upd); err != nil {
		return err
	}

	if plan != "" && status == "active" {
		limit := billing.LimitsFor(plan)
		if !limit.ChannelsUnlimited {
			if err := h.Queries.PauseChannelsAlphabetically(ctx, tx, user.ID, limit.ChannelLimit); err != nil {
				log.Printf("billing webhook: pause excess channels user=%s plan=%s: %v", user.ID, plan, err)
			}
		}

		// 7.4: Send downgrade notification email when plan changes to a lower tier.
		oldPlan := user.SubscriptionPlan
		if oldPlan == "" {
			oldPlan = billing.PlanFree
		}
		if plan != oldPlan && h.Pipeline != nil {
			pausedChannels := false
			if !limit.ChannelsUnlimited {
				pausedChannels = true
			}
			_ = h.Pipeline.Send(ctx, email.Message{
				Template:       email.TemplatePlanDowngrade,
				ToUserID:       user.ID,
				ToAddress:      user.Email,
				IdempotencyKey: "plan_downgrade:" + user.ID + ":" + plan,
				Variables: map[string]any{
					"OldPlan":        oldPlan,
					"NewPlan":        plan,
					"ChannelLimit":   limit.ChannelLimit,
					"PausedChannels": pausedChannels,
				},
			})
		}
	}

	return nil
}

func (h *BillingHandler) handleSubscriptionDeleted(ctx context.Context, tx *sql.Tx, event stripe.Event) error {
	var s stripeSubscriptionEvent
	if err := json.Unmarshal(event.Data.Raw, &s); err != nil {
		return err
	}

	// Resolve user: metadata first, customer id second.
	var user *db.User
	var err error
	if id := s.Metadata["user_id"]; id != "" {
		user, err = h.Queries.GetUserByID(ctx, id)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	if user == nil && s.Customer != "" {
		user, err = h.Queries.GetUserByStripeCustomerID(ctx, s.Customer)
		if err != nil {
			return err
		}
	}
	if user == nil {
		log.Printf("billing webhook: customer.subscription.deleted unknown user (sub=%s)", s.ID)
		return nil
	}

	// Downgrade to free per spec 5.5.
	freePlan := billing.PlanFree
	freeLimits := billing.LimitsFor(freePlan)
	status := "canceled"
	upd := db.SubscriptionUpdate{
		UserID:                    user.ID,
		SubscriptionPlan:          &freePlan,
		CommentQuotaMonthly:       &freeLimits.CommentQuotaMonthly,
		SubscriptionStatus:        &status,
		BillingIntervalClear:      true,
		PastDueSinceClear:         true,
		StripeSubscriptionIDClear: true,
	}
	if err := h.Queries.ApplySubscriptionUpdate(ctx, tx, upd); err != nil {
		return err
	}

	// Free plan keeps 1 channel; pause everything else alphabetically.
	return h.Queries.PauseChannelsAlphabetically(ctx, tx, user.ID, freeLimits.ChannelLimit)
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// priceMapPlanInterval inspects a Stripe Subscription's first item and
// returns the matching (plan, interval) from the configured PriceMap.
func (h *BillingHandler) priceMapPlanInterval(sub *stripe.Subscription) (string, string, bool) {
	if sub == nil || sub.Items == nil || len(sub.Items.Data) == 0 {
		return "", "", false
	}
	item := sub.Items.Data[0]
	if item.Price == nil {
		return "", "", false
	}
	return h.BillingClient.PriceMap().PlanFromPriceID(item.Price.ID)
}

// mapStripeStatus narrows the Stripe subscription status set onto the
// 4-value local state machine.
func mapStripeStatus(s string) string {
	switch s {
	case "active", "trialing":
		return "active"
	case "past_due", "unpaid":
		return "past_due"
	case "canceled", "incomplete_expired":
		return "canceled"
	case "paused":
		return "paused"
	default:
		return "active"
	}
}

// (writeJSON is defined in auth_handlers.go and reused here.)
