// cancel_deletion is the support CLI for reversing a pending account
// deletion when a user contacts ops within the 30-day grace window.
//
// Usage:
//   go run ./tools/cancel_deletion --email user@example.com
//   go run ./tools/cancel_deletion --email user@example.com --confirm
//
// Without --confirm the tool runs in dry-run mode: it loads the user,
// validates the deletion job is still in 'pending' status, and prints
// the proposed action. With --confirm the operations execute atomically
// inside a single transaction:
//
//   1. SELECT the user + deletion job FOR UPDATE
//   2. Verify status='pending' (refuse 'processing'/'completed'/'failed')
//   3. UPDATE users SET deletion_requested_at = NULL
//   4. DELETE FROM account_deletion_jobs WHERE user_id = $1
//
// data-lifecycle (13.1): the explicit --confirm flag is the spec-required
// guard against accidental cancellation. Refusing to proceed when the
// job is already 'processing' is essential — by then the deletion worker
// may have started revoking OAuth tokens or deleting comments, and a
// partial rollback would leave the account in an inconsistent state.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var (
		email   = flag.String("email", "", "email address of the user whose deletion to cancel (required)")
		confirm = flag.Bool("confirm", false, "actually perform the cancellation (default: dry-run)")
		dsn     = flag.String("dsn", os.Getenv("DATABASE_URL"), "Postgres DSN (defaults to $DATABASE_URL)")
	)
	flag.Parse()

	if *email == "" {
		fmt.Fprintln(os.Stderr, "error: --email is required")
		flag.Usage()
		os.Exit(2)
	}
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "error: --dsn or $DATABASE_URL must be set")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	mode := "DRY-RUN"
	if *confirm {
		mode = "EXECUTE"
	}
	fmt.Printf("[%s] cancel_deletion email=%s\n", mode, *email)

	if err := run(ctx, pool, *email, *confirm); err != nil {
		log.Fatalf("FAIL: %v", err)
	}
	fmt.Println("OK")
}

func run(ctx context.Context, pool *pgxpool.Pool, email string, confirm bool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var userID string
	var deletionRequestedAt *time.Time
	err = tx.QueryRow(ctx,
		`SELECT id, deletion_requested_at FROM users WHERE email = $1 FOR UPDATE`,
		email).Scan(&userID, &deletionRequestedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("no user with email %q", email)
	}
	if err != nil {
		return fmt.Errorf("select user: %w", err)
	}
	if deletionRequestedAt == nil {
		return fmt.Errorf("user %s has no pending deletion (deletion_requested_at IS NULL)", userID)
	}

	var jobStatus string
	var jobScheduledFor time.Time
	err = tx.QueryRow(ctx,
		`SELECT status, scheduled_for FROM account_deletion_jobs
		  WHERE user_id = $1 FOR UPDATE`,
		userID).Scan(&jobStatus, &jobScheduledFor)
	if errors.Is(err, pgx.ErrNoRows) {
		// User has deletion_requested_at set but no job row — anomalous,
		// but recoverable by clearing the timestamp alone. Allow it.
		fmt.Printf("note: no account_deletion_jobs row found; will clear users.deletion_requested_at only\n")
		jobStatus = "pending" // synthesize so the gate below passes
	} else if err != nil {
		return fmt.Errorf("select job: %w", err)
	}

	if jobStatus != "pending" {
		return fmt.Errorf("refusing: job status is %q (only 'pending' jobs may be cancelled)", jobStatus)
	}

	fmt.Printf("user_id=%s deletion_requested_at=%s scheduled_for=%s\n",
		userID, deletionRequestedAt.Format(time.RFC3339), jobScheduledFor.Format(time.RFC3339))

	if !confirm {
		fmt.Println("dry-run: not modifying anything (re-run with --confirm)")
		return nil
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET deletion_requested_at = NULL, updated_at = NOW() WHERE id = $1`,
		userID); err != nil {
		return fmt.Errorf("clear deletion_requested_at: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM account_deletion_jobs WHERE user_id = $1`,
		userID); err != nil {
		return fmt.Errorf("delete job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
