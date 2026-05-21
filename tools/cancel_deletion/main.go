package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	var (
		email   = flag.String("email", "", "email address of the user whose deletion to cancel (required)")
		confirm = flag.Bool("confirm", false, "actually perform the cancellation (default: dry-run)")
		dsn     = flag.String("dsn", os.Getenv("DATABASE_PATH"), "SQLite database path (defaults to $DATABASE_PATH)")
	)
	flag.Parse()

	if *email == "" {
		fmt.Fprintln(os.Stderr, "error: --email is required")
		flag.Usage()
		os.Exit(2)
	}
	if *dsn == "" {
		*dsn = "./noumena.db"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database, err := sql.Open("sqlite", *dsn+"?_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate&_fk=true")
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	mode := "DRY-RUN"
	if *confirm {
		mode = "EXECUTE"
	}
	fmt.Printf("[%s] cancel_deletion email=%s\n", mode, *email)

	if err := run(ctx, database, *email, *confirm); err != nil {
		log.Fatalf("FAIL: %v", err)
	}
	fmt.Println("OK")
}

func run(ctx context.Context, database *sql.DB, email string, confirm bool) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	var userID string
	var deletionRequestedAt string
	err = tx.QueryRowContext(ctx,
		`SELECT id, deletion_requested_at FROM users WHERE email = ?`,
		email).Scan(&userID, &deletionRequestedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("no user with email %q", email)
	}
	if err != nil {
		return fmt.Errorf("select user: %w", err)
	}
	if deletionRequestedAt == "" {
		return fmt.Errorf("user %s has no pending deletion (deletion_requested_at IS NULL)", userID)
	}

	var jobStatus string
	var jobScheduledFor string
	err = tx.QueryRowContext(ctx,
		`SELECT status, created_at FROM account_deletion_jobs
		  WHERE user_id = ?`,
		userID).Scan(&jobStatus, &jobScheduledFor)
	if errors.Is(err, sql.ErrNoRows) {
		fmt.Printf("note: no account_deletion_jobs row found; will clear users.deletion_requested_at only\n")
		jobStatus = "pending"
	} else if err != nil {
		return fmt.Errorf("select job: %w", err)
	}

	if jobStatus != "pending" {
		return fmt.Errorf("refusing: job status is %q (only 'pending' jobs may be cancelled)", jobStatus)
	}

	fmt.Printf("user_id=%s deletion_requested_at=%s scheduled_for=%s\n",
		userID, deletionRequestedAt, jobScheduledFor)

	if !confirm {
		fmt.Println("dry-run: not modifying anything (re-run with --confirm)")
		return nil
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET deletion_requested_at = NULL, updated_at = datetime('now') WHERE id = ?`,
		userID); err != nil {
		return fmt.Errorf("clear deletion_requested_at: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM account_deletion_jobs WHERE user_id = ?`,
		userID); err != nil {
		return fmt.Errorf("delete job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
