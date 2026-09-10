// Package billing: trial_reminder.go houses the daily cron that emits
// trial_expiry transactional emails.
//
// data-lifecycle change (12.5): selects users whose trial_ends_at lands
// in the (NOW+2d, NOW+3d] window — exactly one calendar day before the
// 2-day-out reminder window opens — and emits TemplateTrialExpiry. The
// email pipeline's idempotency_key (template+user+day-bucket) collapses
// duplicate sends if the cron runs more than once per day.
//
// The cron is gated by Pipeline != nil. Without an email pipeline there
// is no useful work to do, so the goroutine exits immediately with a log
// line. This mirrors how Section 9 (export worker) gates on Pipeline.
package billing

import (
	"context"
	"log"
	"time"

	"noumena/internal/cronutil"
	"noumena/internal/db"
	"noumena/internal/email"
)

// TrialReminderStore is the narrow surface trial-reminder needs.
// *db.PostgresQuery satisfies it via ListUsersForTrialExpiryReminder.
type TrialReminderStore interface {
	ListUsersForTrialExpiryReminder(ctx context.Context, lo, hi time.Time) ([]db.TrialReminderUser, error)
}

// TrialReminderConfig configures StartTrialReminder. Defaults keep the
// behaviour aligned with spec: daily run at 08:00 UTC, 3-day reminder
// window, idempotent per (user, day).
type TrialReminderConfig struct {
	Interval     time.Duration // default 24h
	InitialDelay time.Duration // default 0
	// LeadTime is how far in advance of trial_ends_at we email. Default
	// 3 days. The select window is (NOW+LeadTime-1d, NOW+LeadTime].
	LeadTime time.Duration
}

// StartTrialReminder launches the daily cron. Exits when ctx is done.
// Returns immediately (no-op) when store or pipeline is nil.
func StartTrialReminder(ctx context.Context, store TrialReminderStore, p *email.Pipeline, cfg TrialReminderConfig) {
	if store == nil || p == nil {
		log.Println("billing trial reminder: store or pipeline nil, not starting")
		return
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.LeadTime <= 0 {
		cfg.LeadTime = 72 * time.Hour
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
			runTrialReminderOnce(ctx, store, p, cfg.LeadTime)
		})
	}()
}

func runTrialReminderOnce(ctx context.Context, store TrialReminderStore, p *email.Pipeline, lead time.Duration) {
	now := time.Now().UTC()
	hi := now.Add(lead)           // upper bound: trial_ends_at <= NOW + 3d
	lo := hi.Add(-24 * time.Hour) // lower bound: 24h earlier (1-day window)

	users, err := store.ListUsersForTrialExpiryReminder(ctx, lo, hi)
	if err != nil {
		log.Printf("billing trial reminder: list err=%v", err)
		return
	}
	if len(users) == 0 {
		return
	}
	log.Printf("billing trial reminder: dispatching %d emails (window %s..%s)",
		len(users), lo.Format(time.RFC3339), hi.Format(time.RFC3339))

	dayBucket := now.Format("2006-01-02")
	for _, u := range users {
		days := int(time.Until(u.TrialEndsAt).Hours()/24) + 1
		if days < 0 {
			days = 0
		}
		msg := email.Message{
			ToUserID:       u.ID,
			ToAddress:      u.Email,
			Template:       email.TemplateTrialExpiry,
			IdempotencyKey: "trial_expiry:" + u.ID + ":" + dayBucket,
			Variables: map[string]any{
				"PlanName":      "Creator",
				"DaysRemaining": days,
				"UpgradeURL":    "",
			},
		}
		if err := p.Send(ctx, msg); err != nil {
			log.Printf("trial_expiry email enqueue failed user=%s: %v", u.ID, err)
		}
	}
}
