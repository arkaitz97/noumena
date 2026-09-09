// Package billing: overage_invoicing.go houses the periodic job that turns
// comment_usage_logs overage rows into Stripe customer-balance debits
// (implement-missing-product-features 8.x).
//
// Exactly-once semantics per (user, period):
//   - Stripe side: the Idempotency-Key header is "overage:<user>:<period>",
//     so Stripe collapses retries of the same charge into one transaction.
//   - Local side: the overage_invoices guard table (migration v13) records
//     each charged (user, period); ListOverageGroups only returns periods
//     with no guard row, so a second run never re-charges.
//
// Free plan is skipped (TracksOverage == false in the SQL join) and Agency
// uses custom pricing handled outside this capability. Rollout is gated by
// config.BillingOverageInvoicingEnabled (Stripe test mode first).
package billing

import (
	"context"
	"log"
	"strconv"
	"time"

	"noumena/internal/db"
)

// OverageInvoicingStore is the narrow DB surface the overage-invoicing job
// needs. *db.SQLiteQuery satisfies it.
type OverageInvoicingStore interface {
	ListOverageGroups(ctx context.Context) ([]db.OverageGroup, error)
	GetUserByID(ctx context.Context, id string) (*db.User, error)
	MarkOverageInvoiced(ctx context.Context, userID, periodStart, stripeInvoiceID string, amountCents int) error
}

// overageIdempotencyKey is the per-(user, period) Stripe idempotency key.
func overageIdempotencyKey(userID string, periodStart time.Time) string {
	return "overage:" + userID + ":" + periodStart.Format(time.RFC3339)
}

func overageDescription(g db.OverageGroup) string {
	return "Comment overage " + strconv.Itoa(g.Overage) + " for period " +
		g.PeriodStart.Format("2006-01-02") + " (plan " + g.Plan + ")"
}

// InvoiceOverage charges every uninvoiced overage period in one pass and
// marks each charged period invoiced. It is safe to call repeatedly: groups
// already present in overage_invoices are excluded by the store, and the
// Stripe idempotency key closes the charge/mark race on retries. Returns
// the number of periods charged (after the store list succeeds). Failures
// are per-group and logged, never fatal to the run.
func InvoiceOverage(ctx context.Context, store OverageInvoicingStore, client Client, enabled bool) error {
	if store == nil || client == nil {
		log.Println("billing overage invoicing: store or client nil, skipping")
		return nil
	}
	if !enabled {
		return nil
	}

	groups, err := store.ListOverageGroups(ctx)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return nil
	}

	invoiced := 0
	for _, g := range groups {
		periodStart := g.PeriodStart.Format(time.RFC3339)
		user, err := store.GetUserByID(ctx, g.UserID)
		if err != nil {
			log.Printf("billing overage: GetUserByID user=%s err=%v", g.UserID, err)
			continue
		}
		if user == nil || user.StripeCustomerID == nil || *user.StripeCustomerID == "" {
			log.Printf("billing overage: skip user=%s period=%s (no Stripe customer)", g.UserID, periodStart)
			continue
		}

		amountCents := OverageAmountCents(g.Overage)
		if amountCents <= 0 {
			log.Printf("billing overage: skip user=%s period=%s overage=%d (amount 0)", g.UserID, periodStart, g.Overage)
			continue
		}

		invoiceID, err := client.CreateCustomerBalanceCharge(
			ctx, *user.StripeCustomerID, int64(amountCents), overageDescription(g),
			overageIdempotencyKey(g.UserID, g.PeriodStart),
			map[string]string{
				"user_id":      g.UserID,
				"period_start": periodStart,
				"overage":      strconv.Itoa(g.Overage),
				"plan":         g.Plan,
			})
		if err != nil {
			log.Printf("billing overage: charge failed user=%s period=%s amount_cents=%d err=%v",
				g.UserID, periodStart, amountCents, err)
			continue
		}

		if err := store.MarkOverageInvoiced(ctx, g.UserID, periodStart, invoiceID, amountCents); err != nil {
			log.Printf("billing overage: mark-invoiced failed user=%s period=%s invoice=%s err=%v",
				g.UserID, periodStart, invoiceID, err)
			continue
		}
		invoiced++
		log.Printf("billing overage: invoiced user=%s period=%s overage=%d amount_cents=%d invoice=%s",
			g.UserID, periodStart, g.Overage, amountCents, invoiceID)
	}
	log.Printf("billing overage invoicing: groups=%d invoiced=%d", len(groups), invoiced)
	return nil
}

// OverageInvoicingConfig configures StartOverageInvoicingCron. Zero values
// use defaults.
type OverageInvoicingConfig struct {
	// Enabled gates the job (config.BillingOverageInvoicingEnabled).
	Enabled bool
	// Interval between runs. Default: 24h.
	Interval time.Duration
	// InitialDelay before the first run. Default: 0 (run immediately).
	InitialDelay time.Duration
}

// StartOverageInvoicingCron launches the daily overage-invoicing job. It is
// a no-op when the store/client is nil or Enabled is false (matches how the
// quota cron and trial reminder gate). Exits when ctx is cancelled.
func StartOverageInvoicingCron(ctx context.Context, store OverageInvoicingStore, client Client, cfg OverageInvoicingConfig) {
	if store == nil || client == nil {
		log.Println("billing overage invoicing: store or client nil, not starting")
		return
	}
	if !cfg.Enabled {
		log.Println("billing overage invoicing: disabled (BILLING_OVERAGE_INVOICING_ENABLED=false), not starting")
		return
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}

	go func() {
		if cfg.InitialDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(cfg.InitialDelay):
			}
		}
		if err := InvoiceOverage(ctx, store, client, true); err != nil {
			log.Printf("billing overage invoicing: run err=%v", err)
		}

		t := time.NewTicker(cfg.Interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := InvoiceOverage(ctx, store, client, true); err != nil {
					log.Printf("billing overage invoicing: run err=%v", err)
				}
			}
		}
	}()
}

// Compile-time assertion that *db.SQLiteQuery satisfies
// OverageInvoicingStore.
var _ OverageInvoicingStore = (*db.SQLiteQuery)(nil)
