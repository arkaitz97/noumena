// Package dbtest provides a Postgres-backed integration test harness for
// Noumena's Go test suite.
//
// # Usage
//
// Tests that need a real, fully-migrated Postgres instance call
// SetupTestDB(t) and receive a *pgxpool.Pool connected to a freshly
// provisioned, isolated database. The harness registers cleanup on
// t.Cleanup so the database is dropped (and the pool closed) when the test
// returns — pass, fail, or panic.
//
//	func TestSomething(t *testing.T) {
//	    pool := dbtest.SetupTestDB(t)
//	    // ... use pool ...
//	}
//
// # Build tag policy
//
// Every test that imports this package MUST carry the build constraint
//
//	//go:build integration
//
// at the top of the file. The default `go test ./...` invocation does not
// set this tag, so harness-dependent tests are not compiled and Docker is
// not required. CI runs `go test -tags=integration ./...` (see
// scripts/test-integration.sh and the `make test-integration` target) to
// execute the integration suite.
//
// # Backends
//
// SetupTestDB has two backends, selected automatically:
//
//  1. External Postgres: if TEST_DATABASE_URL is set to a DSN whose role
//     can CREATE DATABASE, the harness connects there and provisions a
//     unique per-test database against that server. No Docker is used.
//
//  2. Container: otherwise, the harness boots a Postgres 16 container via
//     testcontainers-go (github.com/testcontainers/testcontainers-go),
//     once per test binary. Each test gets a fresh database cloned from a
//     migrated template within that container.
//
// If neither TEST_DATABASE_URL nor a working Docker daemon is available,
// the harness calls t.Skip with a clear message — making the suite a
// no-op in environments that cannot run integration tests.
//
// # Migrations applied once per test binary
//
// The harness applies internal/db/schema.sql exactly once per test binary
// process (guarded by sync.Once) into a template database
// (`noumena_template_<pid>`). Each subsequent SetupTestDB call performs
// `CREATE DATABASE … TEMPLATE …` to clone the migrated schema in
// milliseconds.
//
// # Per-test isolation
//
// Each test gets its own database (not just a transaction or schema),
// which preserves Postgres semantics that require multiple committed
// connections — pg_advisory_lock, SELECT … FOR UPDATE blocking, partial
// unique indexes, and LISTEN/NOTIFY. Tests can call t.Parallel() safely.
//
// # Pool defaults
//
// Connections are configured with statement_timeout=30s and
// lock_timeout=5s by default to prevent runaway tests from hanging CI.
// Override per-test via SetupTestDBWithOptions.
//
// # JWKS mock
//
// The sub-package internal/dbtest/jwksmock provides a lightweight
// HTTP server implementing OIDC discovery + JWKS for testing the
// Google SSO callback handler.
package dbtest
