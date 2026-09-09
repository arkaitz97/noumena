// Package lifecycle: sweeper.go is the out-of-band retry path for
// provider-token revocation. When a deletion job's in-job revoke fails
// (bounded retries — see revoke.go), the error is recorded on the job
// and deletion proceeds; this worker retries the revocation while the
// user's credentials still exist in the DB (job still in 'deleting').
// Idempotent: Google returns 400 for already-invalid tokens, which
// RevokeOAuthToken treats as success.
package lifecycle

import (
	"context"
	"log"
	"sync"
	"time"

	"noumena/internal/db"
)

// RevokeSweeper is a periodic ticker that retries failed provider-token
// revocations for in-flight deletion jobs. Construct via the struct;
// call Start(ctx) once.
type RevokeSweeper struct {
	Queries db.DBQuery
	Logger  *log.Logger

	// Tick controls the poll interval. Defaults to 60s.
	Tick time.Duration

	stopOnce sync.Once
	done     chan struct{}
}

// Start spawns the sweeper loop. Returns immediately; the caller owns ctx.
func (s *RevokeSweeper) Start(ctx context.Context) {
	if s.Logger == nil {
		s.Logger = log.Default()
	}
	if s.Tick == 0 {
		s.Tick = 60 * time.Second
	}
	s.done = make(chan struct{})
	go s.run(ctx)
}

// Stop blocks until the run loop has exited. Safe to call multiple times.
func (s *RevokeSweeper) Stop() {
	s.stopOnce.Do(func() {
		if s.done != nil {
			<-s.done
		}
	})
}

func (s *RevokeSweeper) run(ctx context.Context) {
	defer close(s.done)
	t := time.NewTicker(s.Tick)
	defer t.Stop()
	s.Logger.Printf("revoke_sweeper_started tick=%s", s.Tick)
	for {
		select {
		case <-ctx.Done():
			s.Logger.Printf("revoke_sweeper_stopping err=%v", ctx.Err())
			return
		case <-t.C:
			s.processOne(ctx)
		}
	}
}

func (s *RevokeSweeper) processOne(ctx context.Context) {
	jobs, err := s.Queries.ListJobsPendingRevoke(ctx)
	if err != nil {
		s.Logger.Printf("revoke_sweeper_list_err err=%v", err)
		return
	}
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := s.retryJob(ctx, job); err != nil {
			s.Logger.Printf("revoke_sweeper_retry_failed job=%s user=%s err=%v", job.ID, job.UserID, err)
		}
	}
}

func (s *RevokeSweeper) retryJob(ctx context.Context, job *db.AccountDeletionJob) error {
	channels, err := s.Queries.GetUserTrackingChannels(ctx, job.UserID)
	if err != nil {
		return err
	}
	for _, ch := range channels {
		if err := ctx.Err(); err != nil {
			return err
		}
		if ch.AccessTokenEnc == "" && ch.RefreshTokenEnc == "" {
			continue
		}
		if err := revokeChannelTokens(ctx, ch, s.Logger); err != nil {
			return err
		}
	}
	// All channels revoked (or nothing left to revoke) — clear the error.
	return s.Queries.MarkDeletionRevokeStatus(ctx, job.ID, true, "")
}
