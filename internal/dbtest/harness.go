package dbtest

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"noumena/internal/db"
)

type Options struct {
	StatementTimeout int
	LockTimeout      int
}

func SetupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return SetupTestDBWithOptions(t, Options{})
}

// SetupTestDBWithOptions provisions an isolated in-memory SQLite
// database and applies the REAL migration set (db.Migrate, which runs
// the same embedded schema.sql baseline + versioned migrations the app
// boots with) — not a duplicated schema copy. Tests that pass because
// of the harness's diverged copy will now fail, which is the point:
// the harness must mirror production.
func SetupTestDBWithOptions(t *testing.T, opts Options) *sql.DB {
	t.Helper()

	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("dbtest: open db: %v", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if err := database.Ping(); err != nil {
		t.Fatalf("dbtest: ping db: %v", err)
	}

	// Mirror the pragmas InitDB applies via the DSN.
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("dbtest: set WAL mode: %v", err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA busy_timeout=5000"); err != nil {
		t.Fatalf("dbtest: set busy_timeout: %v", err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("dbtest: enable foreign keys: %v", err)
	}

	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("dbtest: apply migrations: %v", err)
	}

	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("dbtest: close db: %v", err)
		}
	})

	return database
}
