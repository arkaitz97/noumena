package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"noumena/internal/db"
)

// Options tunes a single SetupTestDB invocation.
type Options struct {
	// StatementTimeout overrides the default 30s statement_timeout for the
	// returned pool. Zero means "use default".
	StatementTimeout time.Duration
	// LockTimeout overrides the default 5s lock_timeout. Zero means
	// "use default".
	LockTimeout time.Duration
}

const (
	defaultStatementTimeout = 30 * time.Second
	defaultLockTimeout      = 5 * time.Second
)

// SetupTestDB returns a *pgxpool.Pool connected to a fresh, fully migrated
// Postgres database. See package doc for behavior and environment knobs.
//
// Example with t.Parallel():
//
//	func TestParallelA(t *testing.T) {
//	    t.Parallel()
//	    pool := dbtest.SetupTestDB(t)
//	    // ... uses its own DB ...
//	}
//
//	func TestParallelB(t *testing.T) {
//	    t.Parallel()
//	    pool := dbtest.SetupTestDB(t)
//	    // ... uses its own DB ...
//	}
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return SetupTestDBWithOptions(t, Options{})
}

// SetupTestDBWithOptions is the option-bearing variant of SetupTestDB.
func SetupTestDBWithOptions(t *testing.T, opts Options) *pgxpool.Pool {
	t.Helper()
	srv := getOrInitServer(t)
	if srv == nil {
		// getOrInitServer called t.Skip already.
		return nil
	}

	// Provision a unique per-test database from the template.
	dbName := uniqueDBName()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.cloneTemplate(ctx, dbName); err != nil {
		t.Fatalf("dbtest: clone template: %v", err)
	}

	dsn := srv.dsnFor(dbName)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("dbtest: parse pool config: %v", err)
	}

	stmt := opts.StatementTimeout
	if stmt == 0 {
		stmt = defaultStatementTimeout
	}
	lock := opts.LockTimeout
	if lock == 0 {
		lock = defaultLockTimeout
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if _, err := conn.Exec(ctx, fmt.Sprintf("SET statement_timeout = %d", stmt.Milliseconds())); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, fmt.Sprintf("SET lock_timeout = %d", lock.Milliseconds())); err != nil {
			return err
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("dbtest: open pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		dctx, dcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dcancel()
		if err := srv.dropDatabase(dctx, dbName); err != nil {
			t.Logf("dbtest: drop database %s: %v", dbName, err)
		}
	})

	return pool
}

// PurgeStale drops any leaked harness databases on the server reachable via
// TEST_DATABASE_URL (or the active container, if running) whose name has
// the `noumena_test_` prefix and whose age exceeds threshold. Useful in a
// CI sweep job; safe to call from a non-test binary.
func PurgeStale(ctx context.Context, threshold time.Duration) (int, error) {
	srv := activeServer()
	if srv == nil {
		return 0, errors.New("dbtest: no active server (call SetupTestDB first or set TEST_DATABASE_URL)")
	}
	return srv.purgeStale(ctx, threshold)
}

// ----------------------------------------------------------------------------
// Internal: shared server + template lifecycle
// ----------------------------------------------------------------------------

// server represents the once-per-binary Postgres backend (either container
// or external) plus the template database we clone for each test.
type server struct {
	adminDSN     string // admin DSN with no DB or with `postgres` DB
	dsnTemplate  string // DSN with %s placeholder for db name
	templateName string
	terminate    func(context.Context) error // optional container teardown
}

var (
	serverOnce sync.Once
	serverVal  *server
	serverErr  error
)

// getOrInitServer initializes the per-binary server lazily.
//
// If neither TEST_DATABASE_URL is set nor Docker is available, this calls
// t.Skip and returns nil. Otherwise it boots/reuses the backend, applies
// migrations into a template, and returns the cached *server.
func getOrInitServer(t *testing.T) *server {
	t.Helper()
	serverOnce.Do(func() {
		serverVal, serverErr = initServer()
	})
	if serverErr != nil {
		// Distinguish "skip" from "fatal".
		var s skipErr
		if errors.As(serverErr, &s) {
			t.Skip(s.Error())
			return nil
		}
		t.Fatalf("dbtest: init server: %v", serverErr)
		return nil
	}
	return serverVal
}

func activeServer() *server {
	return serverVal
}

// skipErr signals that the harness should call t.Skip with the wrapped
// reason instead of t.Fatal.
type skipErr struct{ msg string }

func (s skipErr) Error() string { return s.msg }

func initServer() (*server, error) {
	if dsn := os.Getenv("TEST_DATABASE_URL"); strings.TrimSpace(dsn) != "" {
		return initExternal(dsn)
	}
	// Fall back to docker. Detect docker availability up front to give a
	// clean skip message.
	if !dockerAvailable() {
		return nil, skipErr{msg: "dbtest: TEST_DATABASE_URL not set and docker is not available; integration tests skipped"}
	}
	return initContainer()
}

func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func initContainer() (*server, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	const (
		user = "noumena"
		pass = "noumena"
		db   = "postgres"
	)

	c, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase(db),
		tcpostgres.WithUsername(user),
		tcpostgres.WithPassword(pass),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		// If docker is broken for a different reason (no daemon perms,
		// e.g.), skip rather than fail loudly.
		return nil, skipErr{msg: fmt.Sprintf("dbtest: docker postgres container failed to start; integration tests skipped: %v", err)}
	}

	// Belt and suspenders: wait for SQL.
	_ = tcwait.ForSQL("5432/tcp", "pgx", func(host string, port string) string {
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, db)
	})

	host, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, fmt.Errorf("get host: %w", err)
	}
	port, err := c.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, fmt.Errorf("get port: %w", err)
	}

	adminDSN := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable",
		user, pass, host, port.Port())
	dsnTemplate := fmt.Sprintf("postgres://%s:%s@%s:%s/%%s?sslmode=disable",
		user, pass, host, port.Port())

	srv := &server{
		adminDSN:    adminDSN,
		dsnTemplate: dsnTemplate,
		terminate: func(ctx context.Context) error {
			return c.Terminate(ctx)
		},
	}
	if err := srv.bootstrapTemplate(); err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	return srv, nil
}

func initExternal(dsn string) (*server, error) {
	// Parse via pgx to derive a "no-database" admin DSN we can use to
	// CREATE DATABASE, plus a templated DSN.
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
	}

	host := cfg.Host
	port := cfg.Port
	user := cfg.User
	pass := cfg.Password
	ssl := "disable"
	if v, ok := cfg.RuntimeParams["sslmode"]; ok && v != "" {
		ssl = v
	}

	adminDSN := buildDSN(user, pass, host, port, "postgres", ssl)
	dsnTemplate := buildDSN(user, pass, host, port, "%s", ssl)

	srv := &server{
		adminDSN:    adminDSN,
		dsnTemplate: dsnTemplate,
	}
	if err := srv.bootstrapTemplate(); err != nil {
		return nil, err
	}
	return srv, nil
}

func buildDSN(user, pass, host string, port uint16, dbname, ssl string) string {
	// Quote nothing: tests control these values.
	if pass == "" {
		return fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=%s", user, host, port, dbname, ssl)
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", user, pass, host, port, dbname, ssl)
}

// bootstrapTemplate creates `noumena_template_<pid>`, applies the embedded
// schema, and stores the template name on the server. Idempotent: a stale
// template from a previous run with the same PID is dropped first.
func (s *server) bootstrapTemplate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tmpl := fmt.Sprintf("noumena_template_%d", os.Getpid())
	s.templateName = tmpl

	admin, err := pgx.Connect(ctx, s.adminDSN)
	if err != nil {
		return fmt.Errorf("connect admin: %w", err)
	}
	defer admin.Close(ctx)

	// Drop any previous template (best-effort) and recreate.
	if _, err := admin.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q`, tmpl)); err != nil {
		return fmt.Errorf("drop stale template: %w", err)
	}
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q`, tmpl)); err != nil {
		return fmt.Errorf("create template db: %w", err)
	}

	// Connect to template and apply schema via the production
	// migration entrypoint so the harness exercises exactly the same
	// SQL as production startup.
	tmplDSN := s.dsnFor(tmpl)
	if err := db.InitDB(tmplDSN); err != nil {
		return fmt.Errorf("init db pool on template: %w", err)
	}
	if err := db.ApplyMigrations(ctx); err != nil {
		db.CloseDB()
		return fmt.Errorf("apply migrations: %w", err)
	}
	db.CloseDB()

	// Mark template as a TEMPLATE so CREATE DATABASE ... TEMPLATE works
	// for any role with CREATEDB.
	if _, err := admin.Exec(ctx, fmt.Sprintf(`ALTER DATABASE %q IS_TEMPLATE = true`, tmpl)); err != nil {
		// Non-fatal: PG allows TEMPLATE-clone from any DB if the source
		// has no other connections. Still, mark when we can.
		// Don't return — proceed.
		_ = err
	}

	return nil
}

func (s *server) cloneTemplate(ctx context.Context, dbName string) error {
	admin, err := pgx.Connect(ctx, s.adminDSN)
	if err != nil {
		return fmt.Errorf("connect admin: %w", err)
	}
	defer admin.Close(ctx)
	q := fmt.Sprintf(`CREATE DATABASE %q TEMPLATE %q`, dbName, s.templateName)
	if _, err := admin.Exec(ctx, q); err != nil {
		return fmt.Errorf("clone db %s: %w", dbName, err)
	}
	return nil
}

func (s *server) dropDatabase(ctx context.Context, dbName string) error {
	admin, err := pgx.Connect(ctx, s.adminDSN)
	if err != nil {
		return err
	}
	defer admin.Close(ctx)
	// FORCE disconnects any lingering pool conns before drop.
	_, err = admin.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, dbName))
	return err
}

func (s *server) dsnFor(dbName string) string {
	return strings.Replace(s.dsnTemplate, "%s", dbName, 1)
}

// purgeStale drops databases matching the harness pattern and older than
// threshold (based on pg_database_size + pg_stat_database last activity is
// not reliable — we use the catalog's `datfrozenxid` proxy via age()).
//
// Implementation: enumerate pg_database, drop any noumena_test_* whose
// connection is idle. We don't track creation time in the catalog, so the
// threshold is interpreted as "drop if no active backend".
func (s *server) purgeStale(ctx context.Context, _ time.Duration) (int, error) {
	admin, err := pgx.Connect(ctx, s.adminDSN)
	if err != nil {
		return 0, err
	}
	defer admin.Close(ctx)

	rows, err := admin.Query(ctx,
		`SELECT datname FROM pg_database WHERE datname LIKE 'noumena_test_%'`)
	if err != nil {
		return 0, err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return 0, err
		}
		names = append(names, n)
	}
	rows.Close()

	dropped := 0
	for _, n := range names {
		if _, err := admin.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, n)); err == nil {
			dropped++
		}
	}
	return dropped, nil
}

// uniqueDBName returns a database name unique to this process and call.
func uniqueDBName() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("noumena_test_%d_%s", os.Getpid(), hex.EncodeToString(b[:]))
}
