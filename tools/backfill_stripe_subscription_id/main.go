package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	_ "modernc.org/sqlite"
	"github.com/stripe/stripe-go/v76"
	stripesub "github.com/stripe/stripe-go/v76/subscription"
)

func main() {
	var (
		apply   = flag.Bool("apply", false, "actually write stripe_subscription_id (default: dry-run)")
		dryRun  = flag.Bool("dry-run", true, "explicit dry-run flag (ignored when --apply is set)")
		dsn     = flag.String("dsn", os.Getenv("DATABASE_PATH"), "SQLite database path (defaults to $DATABASE_PATH)")
		stripeK = flag.String("stripe-key", os.Getenv("STRIPE_SECRET_KEY"), "Stripe secret key (defaults to $STRIPE_SECRET_KEY)")
	)
	flag.Parse()
	_ = dryRun

	if *dsn == "" {
		*dsn = "./noumena.db"
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

	database, err := sql.Open("sqlite", *dsn+"?_journal_mode=WAL&_busy_timeout=5000&_fk=true")
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	summary, err := run(ctx, database, lookupActiveSubscription, *apply)
	if err != nil {
		log.Fatalf("FAIL: %v", err)
	}
	fmt.Printf("summary: %d candidates, %d backfilled, %d had no Stripe subscription\n",
		summary.Candidates, summary.Backfilled, summary.NoSubscription)
}

type Summary struct {
	Candidates     int
	Backfilled     int
	NoSubscription int
}

type candidate struct {
	UserID       string
	CustomerID   string
	Subscription string
}

type subscriptionLookup func(ctx context.Context, customerID string) (string, error)

type dbExec interface {
	QueryContext(ctx context.Context, sql string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, sql string, args ...any) (sql.Result, error)
}

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
		result, err := pool.ExecContext(ctx,
			`UPDATE users SET stripe_subscription_id = ?, updated_at = datetime('now')
			  WHERE id = ? AND stripe_subscription_id IS NULL`,
			subID, c.UserID)
		if err != nil {
			return s, fmt.Errorf("update user %s: %w", c.UserID, err)
		}
		if n, _ := result.RowsAffected(); n == 1 {
			s.Backfilled++
		} else {
			fmt.Printf("  (skipped: stripe_subscription_id already set by concurrent writer)\n")
		}
	}
	return s, nil
}

func loadCandidates(ctx context.Context, pool dbExec) ([]candidate, error) {
	rows, err := pool.QueryContext(ctx,
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
	return out, nil
}

func lookupActiveSubscription(ctx context.Context, customerID string) (string, error) {
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
		var serr *stripe.Error
		if asStripeError(err, &serr) {
			return "", fmt.Errorf("stripe list active: %s", serr.Msg)
		}
		return "", err
	}

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
