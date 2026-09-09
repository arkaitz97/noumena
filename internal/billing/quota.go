package billing

import (
	"context"
	"log"
	"time"

	"noumena/internal/db"
	"noumena/internal/email"
)

// QuotaCronStore is the narrow surface the daily billing crons need.
// *db.PostgresQuery satisfies it.
type QuotaCronStore interface {
	// RecordCurrentPeriodUsage snapshots the current period's usage into
	// billing_usage for every user with a quota schedule (3.1). Must run
	// BEFORE ResetFreeTierQuotas so a just-finished period's usage is
	// captured before the reset zeroes it.
	RecordCurrentPeriodUsage(ctx context.Context) (int64, error)
	ResetFreeTierQuotas(ctx context.Context) (int64, error)
	ExpirePastDueGrace(ctx context.Context, graceHours int) (int64, error)
}

// QuotaCronGraceEmailer is the optional richer surface used when an email
// pipeline is configured: it returns the (id, email) of users transitioned
// to paused so the cron can emit payment_grace_final (12.4). When the
// store satisfies this interface AND a Pipeline is provided, the cron
// uses this path instead of the count-only ExpirePastDueGrace.
type QuotaCronGraceEmailer interface {
	ExpirePastDueGraceWithUsers(ctx context.Context, graceHours int) ([]db.ExpiredGraceUser, error)
}

// QuotaCronConfig configures StartQuotaCron. Zero values use defaults.
type QuotaCronConfig struct {
	// Interval between runs. Default: 24h.
	Interval time.Duration
	// InitialDelay before the first run. Default: 0 (run immediately).
	InitialDelay time.Duration
	// PastDueGraceHours is the time-in-past_due budget before a user is
	// transitioned to paused by ExpirePastDueGrace. Default: 72h (3 days).
	PastDueGraceHours int
	// Pipeline is optional. When set together with a store satisfying
	// QuotaCronGraceEmailer, transitioned users receive payment_grace_final.
	Pipeline *email.Pipeline
}

// StartQuotaCron launches the daily billing maintenance job which
//  1. resets free-tier users whose quota_reset_at is in the past, and
//  2. transitions past_due users to paused once they have been past_due
//     for more than PastDueGraceHours.
//
// The goroutine exits when ctx is cancelled.
func StartQuotaCron(ctx context.Context, store QuotaCronStore, cfg QuotaCronConfig) {
	if store == nil {
		log.Println("billing quota cron: no store, not starting")
		return
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.PastDueGraceHours <= 0 {
		cfg.PastDueGraceHours = 72
	}

	go func() {
		// Optional initial delay — used by main.go to align with 08:00 UTC.
		if cfg.InitialDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(cfg.InitialDelay):
			}
		}
		runQuotaCronOnce(ctx, store, cfg.PastDueGraceHours, cfg.Pipeline)

		t := time.NewTicker(cfg.Interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runQuotaCronOnce(ctx, store, cfg.PastDueGraceHours, cfg.Pipeline)
			}
		}
	}()
}

// runQuotaCronOnce performs one iteration. It logs counts at INFO and
// errors at WARN; failures do not abort the loop.
func runQuotaCronOnce(ctx context.Context, store QuotaCronStore, graceHours int, p *email.Pipeline) {
	// 3.1: snapshot current-period usage before any reset zeroes it.
	if n, err := store.RecordCurrentPeriodUsage(ctx); err != nil {
		log.Printf("billing quota cron: RecordCurrentPeriodUsage err=%v", err)
	} else if n > 0 {
		log.Printf("billing quota cron: billing_usage rows=%d", n)
	}

	if n, err := store.ResetFreeTierQuotas(ctx); err != nil {
		log.Printf("billing quota cron: ResetFreeTierQuotas err=%v", err)
	} else if n > 0 {
		log.Printf("billing quota cron: reset_free_tier_quotas rows=%d", n)
	}

	// 12.4: prefer the email-aware path when both a Pipeline and a store
	// that supports RETURNING are available; fall back to the count-only
	// transition otherwise (preserves existing behaviour for tests/mocks).
	if emitter, ok := store.(QuotaCronGraceEmailer); ok && p != nil {
		users, err := emitter.ExpirePastDueGraceWithUsers(ctx, graceHours)
		if err != nil {
			log.Printf("billing quota cron: ExpirePastDueGraceWithUsers err=%v", err)
			return
		}
		if len(users) > 0 {
			log.Printf("billing quota cron: expire_past_due_grace rows=%d hours=%d (with email)",
				len(users), graceHours)
		}
		for _, u := range users {
			msg := email.Message{
				ToUserID:       u.ID,
				ToAddress:      u.Email,
				Template:       email.TemplatePaymentGraceFinal,
				IdempotencyKey: "payment_grace_final:" + u.ID,
				Variables: map[string]any{
					"PortalURL": "",
				},
			}
			if err := p.Send(ctx, msg); err != nil {
				log.Printf("payment_grace_final email enqueue failed user=%s: %v", u.ID, err)
			}
		}
		return
	}

	if n, err := store.ExpirePastDueGrace(ctx, graceHours); err != nil {
		log.Printf("billing quota cron: ExpirePastDueGrace err=%v", err)
	} else if n > 0 {
		log.Printf("billing quota cron: expire_past_due_grace rows=%d hours=%d", n, graceHours)
	}
}

// untilNextDailyUTC returns the duration from now until the next occurrence
// of (hour, minute) UTC. Useful for callers that want a once-a-day cron at
// a fixed wall-clock time (default 08:00 UTC per spec 8.3).
func untilNextDailyUTC(now time.Time, hour, minute int) time.Duration {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

// NextRunDelayUTC is the public helper used by main.go to align the first
// run of the cron at 08:00 UTC.
func NextRunDelayUTC(hour, minute int) time.Duration {
	return untilNextDailyUTC(time.Now(), hour, minute)
}

// Compile-time assertion that *db.SQLiteQuery satisfies QuotaCronStore.
var _ QuotaCronStore = (*db.SQLiteQuery)(nil)
