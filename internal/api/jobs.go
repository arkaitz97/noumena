package api

import (
	"context"
	"log"
	"time"

	"noumena/internal/config"
	"noumena/internal/cronutil"
	"noumena/internal/db"
	"noumena/internal/email"
)

// StartTokenExpiryNotifier launches a daily ticker that scans for OAuth
// tokens nearing expiry and emails their owner exactly once per cycle.
func StartTokenExpiryNotifier(ctx context.Context, q db.DBQuery, mail email.EmailSender, sched TokenExpirySchedule) {
	if q == nil || mail == nil {
		log.Println("[token-expiry] missing dependencies, job not started")
		return
	}
	if sched.RunAt.IsZero() {
		// Default: 06:00 UTC.
		now := time.Now().UTC()
		sched.RunAt = time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, time.UTC)
	}
	if sched.Interval <= 0 {
		sched.Interval = 24 * time.Hour
	}

	go cronutil.RunCron(ctx, sched.Interval, true, func(ctx context.Context) {
		runTokenExpiryScan(ctx, q, mail)
	})
}

// TokenExpirySchedule controls the daily scan timing. Defaults are 06:00 UTC
// and a 24h interval.
type TokenExpirySchedule struct {
	RunAt    time.Time
	Interval time.Duration
}

func runTokenExpiryScan(ctx context.Context, q db.DBQuery, mail email.EmailSender) {
	warning := config.Default.TokenExpiryWarningDays
	if warning <= 0 {
		warning = 7
	}

	rows, err := q.GetExpiringTokens(ctx, warning)
	if err != nil {
		log.Printf("[token-expiry] GetExpiringTokens failed: %v", err)
		return
	}

	notified := 0
	for _, row := range rows {
		user, err := q.GetUserByID(ctx, row.UserID)
		if err != nil || user == nil {
			log.Printf("[token-expiry] user lookup failed for %s: %v", row.UserID, err)
			continue
		}
		channel, err := q.GetChannelByPlatformID(ctx, "youtube", row.PlatformChannelID)
		// If lookup by platform_channel_id (UUID) fails, try direct row scan via list.
		_ = channel
		var displayName string
		// Best-effort: pull the display name via the user's listing.
		items, err := q.ListUserChannels(ctx, row.UserID)
		if err == nil {
			for _, it := range items {
				if it.ID == row.PlatformChannelID {
					displayName = it.DisplayName
					break
				}
			}
		}
		expires := time.Time{}
		if row.TokenExpiresAt != nil {
			expires = *row.TokenExpiresAt
		}
		settingsURL := config.Default.AppBaseURL + "/settings/channels"
		if err := mail.Send(ctx, email.Message{
			Template:       email.TemplateTokenExpiry,
			ToAddress:      user.Email,
			IdempotencyKey: email.IdempotencyKey(email.TemplateTokenExpiry, user.Email, displayName, expires.UTC().Format("2006-01-02")),
			Variables: map[string]any{
				"ChannelName": displayName,
				"ExpiresAt":   expires.UTC().Format("January 2, 2006"),
				"SettingsURL": settingsURL,
			},
		}); err != nil {
			log.Printf("[token-expiry] send failed for tracking %s: %v", row.ID, err)
			continue
		}
		if err := q.MarkTokenExpiryNotified(ctx, row.ID); err != nil {
			log.Printf("[token-expiry] mark notified failed for %s: %v", row.ID, err)
			continue
		}
		notified++
	}
	log.Printf("[token-expiry] scan complete: %d tokens notified out of %d candidates", notified, len(rows))
}

// StartChannelPurgeJob launches a recurring job that scans platform_channels
// with a deletion_scheduled_at <= NOW() and purges them if they remain
// trackerless. The job is idempotent and safe to invoke at any cadence.
func StartChannelPurgeJob(ctx context.Context, q db.DBQuery, interval time.Duration) {
	if q == nil {
		return
	}
	if interval <= 0 {
		interval = time.Hour
	}
	go cronutil.RunCron(ctx, interval, false, func(ctx context.Context) {
		runChannelPurge(ctx, q)
	})
}

func runChannelPurge(ctx context.Context, q db.DBQuery) {
	// We rely on PurgeChannelData to re-check active-tracker count inside a
	// single transaction. The set of candidates is selected via the legacy
	// pgx pool through ListUserChannels / GetChannelsForSync isn't suitable;
	// instead, scan the channels table directly via a dedicated query
	// implemented as raw SQL through ListChannelsForUser is also unsuitable.
	// We expose this scan via a small helper on the pool: callers wishing
	// to use a mock should override the entire job.
	pool, ok := q.(interface {
		ChannelsDueForPurge(ctx context.Context) ([]string, error)
	})
	if !ok {
		// No-op when running against a mock.
		return
	}
	ids, err := pool.ChannelsDueForPurge(ctx)
	if err != nil {
		log.Printf("[purge] list failed: %v", err)
		return
	}
	for _, id := range ids {
		if err := q.PurgeChannelData(ctx, id); err != nil {
			log.Printf("[purge] failed for channel %s: %v", id, err)
		}
	}
}
