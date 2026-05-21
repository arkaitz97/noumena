// Package dbtest provides a SQLite-backed integration test harness for
// Noumena's Go test suite.
//
// # Usage
//
// Tests that need a real, fully-migrated SQLite instance call
// SetupTestDB(t) and receive a Harness with a *sql.DB connected to a freshly
// provisioned, in-memory database. The harness registers cleanup on
// t.Cleanup so the database is closed when the test returns — pass, fail, or panic.
//
//	func TestSomething(t *testing.T) {
//	    harness := dbtest.SetupTestDB(t)
//	    // ... use harness.DB ...
//	}
//
// # Build tag policy
//
// Every test that imports this package MUST carry the build constraint
//
//	//go:build integration
//
// at the top of the file. The default `go test ./...` invocation does not
// set this tag, so harness-dependent tests are not compiled.
// CI runs `go test -tags=integration ./...` to execute the integration suite.
//
// # Backend
//
// SetupTestDB creates an in-memory SQLite database and applies the full
// schema from internal/db/schema.sql. Each test gets its own isolated
// database. No Docker or external services are required.
//
// # Migrations applied per test
//
// The harness applies internal/db/schema.sql to each new in-memory database.
// Since SQLite databases are file-based (or in-memory), each test gets a
// fresh schema with no shared state.
//
// # Per-test isolation
//
// Each test gets its own in-memory SQLite database, which provides complete
// isolation. Tests can call t.Parallel() safely.
//
// # JWKS mock
//
// The sub-package internal/dbtest/jwksmock provides a lightweight
// HTTP server implementing OIDC discovery + JWKS for testing the
// Google SSO callback handler.
package dbtest
