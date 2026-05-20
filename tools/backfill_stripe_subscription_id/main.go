// backfill_stripe_subscription_id is the one-shot operator CLI that
// populates the new `users.stripe_subscription_id` column for accounts
// that were upgraded BEFORE the column existed (proposal:
// backend-schema-and-env-fixes).
//
// Usage:
//
//	go run ./tools/backfill_stripe_subscription_id              # dry-run (default)
//	go run ./tools/backfill_stripe_subscription_id --apply      # actually write
//	go run ./tools/backfill_stripe_subscription_id --apply --dsn=...
//
// Behaviour:
//
//  1. SELECT users WHERE stripe_customer_id IS NOT NULL
//                    AND stripe_subscription_id IS NULL.
//  2. For each user, ask Stripe `subscriptions.List({customer, status:"active"})`.
//     If empty, fall back to `status:"all"` and pick the most recently
//     created (per design.md task 4 mitigation: filter active first to
//     avoid resurrecting canceled subs).
//  3. Dry-run: print one line per row, no writes.
//  4. Apply: `UPDATE users SET stripe_subscription_id=$1 WHERE id=$2 AND
//     stripe_subscription_id IS NULL` — defensive WHERE so a concurrent
//     webhook firing during the backfill doesn't lose its write.
//
// Idempotent: re-running after a successful --apply is a no-op (the
// SELECT filter and the defensive WHERE both guarantee this).
//
// Stripe price/secret config is sourced from environment variables — we
// do NOT load the full server `config.Config` because the backfill is
// allowed to run with `BILLING_ENABLED=false` (the operator just needs
// `STRIPE_SECRET_KEY` and `DATABASE_URL`).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stripe/stripe-go/v76"
	stripesub "github.com/stripe/stripe-go/v76/subscription"
)

func main() {
	var (
		apply   = flag.Bool("apply", false, "actually write stripe_subscription_id (default: dry-run)")
		dryRun  = flag.Bool("dry-run", true, "explicit dry-run flag (ignored when --apply is set)")
		dsn     = flag.String("dsn", os.Getenv("DATABASE_URL"), "Postgres DSN (defaults to $DATABASE_URL)")
		stripeK = flag.String("stripe-key", os.Getenv("STRIPE_SECRET_KEY"), "Stripe secret key (defaults to $STRIPE_SECRET_KEY)")
	)
	flag.Parse()
	_ = dryRun // present for documentation; --apply is the source of truth

	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "error: --dsn or $DATABASE_URL must be set")
		os.Exit(2)
	}
	if *stripeK == "" {
		fmt.Fprintln(os.Stderr, "error: --stripe-key or $STRIPE_SECRET_KEY must be set")
		os.Exit(2)
	}
	stripe.Key = *stripeK

	mode := "DRY-RUN"
	if *apply {
		mode = "APPLY"
	}
	fmt.Printf("[%s] backfill_stripe_subscription_id\n", mode)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	summary, err := run(ctx, pool, lookupActiveSubscription, *apply)
	if err != nil {
		log.Fatalf("FAIL: %v", err)
	}
	fmt.Printf("summary: %d candidates, %d backfilled, %d had no Stripe subscription\n",
		summary.Candidates, summary.Backfilled, summary.NoSubscription)
}

// Summary aggregates the per-row outcomes for the operator.
type Summary struct {
	Candidates     int
	Backfilled     int
	NoSubscription int
}

// candidate is one user row that needs a backfill decision.
type candidate struct {
	UserID       string
	CustomerID   string
	Subscription string // empty if Stripe has none for this customer
}

// subscriptionLookup is the hook used by tests to avoid hitting Stripe.
type subscriptionLookup func(ctx context.Context, customerID string) (string, error)

// dbExec is the minimal subset of pgxpool.Pool that `run` uses; abstracted
// to a tiny interface so unit tests can exercise the logic without a real
// database. Production code passes `*pgxpool.Pool` directly.
type dbExec interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// run is the testable core: it accepts an injected subscriptionLookup
// so unit tests can simulate Stripe responses without network access.
func run(ctx context.Context, pool dbExec, lookup subscriptionLookup, apply bool) (*Summary, error) {
	candidates, err := loadCandidates(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("load candidates: %w", err)
	}

	s := &Summary{Candidates: len(candidates)}
	for _, c := range candidates {
		subID, err := lookup(ctx, c.CustomerID)
		if err != nil {
			fmt.Printf("user=%s customer=%s ERROR: %v\n", c.UserID, c.CustomerID, err)
			continue
		}
		if subID == "" {
			fmt.Printf("user=%s customer=%s subscription=<none>\n", c.UserID, c.CustomerID)
			s.NoSubscription++
			continue
		}
		fmt.Printf("user=%s customer=%s subscription=%s\n", c.UserID, c.CustomerID, subID)
		if !apply {
			continue
		}
		// Defensive WHERE: another process (webhook handler running
		// concurrently with the backfill) may have set the column
		// already. Don't clobber.
		tag, err := pool.Exec(ctx,
			`UPDATE users SET stripe_subscription_id = $1, updated_at = NOW()
			  WHERE id = $2 AND stripe_subscription_id IS NULL`,
			subID, c.UserID)
		if err != nil {
			return s, fmt.Errorf("update user %s: %w", c.UserID, err)
		}
		if tag.RowsAffected() == 1 {
			s.Backfilled++
		} else {
			fmt.Printf("  (skipped: stripe_subscription_id already set by concurrent writer)\n")
		}
	}
	return s, nil
}

func loadCandidates(ctx context.Context, pool dbExec) ([]candidate, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, stripe_customer_id FROM users
		  WHERE stripe_customer_id IS NOT NULL
		    AND stripe_subscription_id IS NULL
		  ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.UserID, &c.CustomerID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// lookupActiveSubscription is the production subscriptionLookup.
//
// Stripe's `subscriptions.List` sorts most-recent-first by default. We
// page only enough to find the first `active` (or, failing that, the
// most recent in any state — the operator can audit dry-run output
// before applying).
func lookupActiveSubscription(ctx context.Context, customerID string) (string, error) {
	// Pass 1: status=active.
	listParams := &stripe.SubscriptionListParams{
		Customer: stripe.String(customerID),
		Status:   stripe.String("active"),
	}
	listParams.Context = ctx
	listParams.Limit = stripe.Int64(1)

	iter := stripesub.List(listParams)
	for iter.Next() {
		sub := iter.Subscription()
		if sub != nil {
			return sub.ID, nil
		}
	}
	if err := iter.Err(); err != nil {
		// Stripe returned an error for the active query; surface it.
		// We do NOT fall through to status=all in this case — an API
		// error is operator-actionable.
		var serr *stripe.Error
		if asStripeError(err, &serr) {
			return "", fmt.Errorf("stripe list active: %s", serr.Msg)
		}
		return "", err
	}

	// Pass 2: status=all, pick most recent.
	allParams := &stripe.SubscriptionListParams{
		Customer: stripe.String(customerID),
		Status:   stripe.String("all"),
	}
	allParams.Context = ctx
	allParams.Limit = stripe.Int64(10)

	iter2 := stripesub.List(allParams)
	var subs []*stripe.Subscription
	for iter2.Next() {
		if s := iter2.Subscription(); s != nil {
			subs = append(subs, s)
		}
	}
	if err := iter2.Err(); err != nil {
		return "", err
	}
	if len(subs) == 0 {
		return "", nil
	}
	sort.Slice(subs, func(i, j int) bool { return subs[i].Created > subs[j].Created })
	return subs[0].ID, nil
}

// asStripeError is a tiny errors.As wrapper so tests don't need to pull
// in the errors package.
func asStripeError(err error, target **stripe.Error) bool {
	for e := err; e != nil; {
		if se, ok := e.(*stripe.Error); ok {
			*target = se
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := e.(unwrapper)
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

// guard against accidental removal of the pgx import when refactoring.
var _ = pgx.ErrNoRows
