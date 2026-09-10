package billing

import (
	"context"
	"fmt"
	"log"
	"time"

	"noumena/internal/cronutil"
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
		if cfg.InitialDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(cfg.InitialDelay):
			}
		}
		cronutil.RunCron(ctx, cfg.Interval, true, func(ctx context.Context) {
			runQuotaCronOnce(ctx, store, cfg.PastDueGraceHours, cfg.Pipeline)
		})
	}()
}

// runQuotaCronOnce performs one iteration. It logs counts at INFO and
// errors at WARN; failures do not abort the loop.
func runQuotaCronOnce(ctx context.Context, store QuotaCronStore, graceHours int, p *email.Pipeline) {
	// 3.1: snapshot current-period usage before any reset zeroes it.
	n, err := store.RecordCurrentPeriodUsage(ctx)
	runQuotaStep("RecordCurrentPeriodUsage", "billing_usage", n, err)

	n, err = store.ResetFreeTierQuotas(ctx)
	runQuotaStep("ResetFreeTierQuotas", "reset_free_tier_quotas", n, err)

	// 12.4: prefer the email-aware path when both a Pipeline and a store
	// that supports RETURNING are available; fall back to the count-only
	// transition otherwise (preserves existing behaviour for tests/mocks).
	if emitter, ok := store.(QuotaCronGraceEmailer); ok && p != nil {
		expirePastDueGraceWithEmail(ctx, emitter, p, graceHours)
		return
	}
	n, err = store.ExpirePastDueGrace(ctx, graceHours)
	runQuotaStep("ExpirePastDueGrace", fmt.Sprintf("expire_past_due_grace hours=%d", graceHours), n, err)
}

// runQuotaStep logs one count-returning cron step: err at WARN, non-zero
// rows at INFO.
func runQuotaStep(errLabel, rowsLabel string, n int64, err error) {
	if err != nil {
		log.Printf("billing quota cron: %s err=%v", errLabel, err)
		return
	}
	if n > 0 {
		log.Printf("billing quota cron: %s rows=%d", rowsLabel, n)
	}
}

func expirePastDueGraceWithEmail(ctx context.Context, emitter QuotaCronGraceEmailer, p *email.Pipeline, graceHours int) {
	users, err := emitter.ExpirePastDueGraceWithUsers(ctx, graceHours)
	if err != nil {
		log.Printf("billing quota cron: ExpirePastDueGraceWithUsers err=%v", err)
		return
	}
	logQuotaEmailRows(len(users), graceHours)
	for _, u := range users {
		sendGraceFinalEmail(ctx, p, u)
	}
}

func logQuotaEmailRows(n, graceHours int) {
	if n > 0 {
		log.Printf("billing quota cron: expire_past_due_grace rows=%d hours=%d (with email)", n, graceHours)
	}
}

func sendGraceFinalEmail(ctx context.Context, p *email.Pipeline, u db.ExpiredGraceUser) {
	msg := email.Message{
		ToUserID:       u.ID,
		ToAddress:      u.Email,
		Template:       email.TemplatePaymentGraceFinal,
		IdempotencyKey: "payment_grace_final:" + u.ID,
		Variables:      map[string]any{"PortalURL": ""},
	}
	if err := p.Send(ctx, msg); err != nil {
		log.Printf("payment_grace_final email enqueue failed user=%s: %v", u.ID, err)
	}
}

// NextRunDelayUTC is the public helper used by main.go to align the first
// run of the cron at 08:00 UTC.
func NextRunDelayUTC(hour, minute int) time.Duration {
	return cronutil.UntilNextDailyUTC(time.Now(), hour, minute)
}

// Compile-time assertion that *db.SQLiteQuery satisfies QuotaCronStore.
var _ QuotaCronStore = (*db.SQLiteQuery)(nil)
