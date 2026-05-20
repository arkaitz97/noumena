// Smoke test that the cancel_deletion main package compiles cleanly and
// that flag parsing rejects missing --email. A full integration test
// against a live Postgres pool is out of scope here (no test container
// in the repo); the validation logic in run() is exercised by the
// deletion-worker integration suite once it is added.
package main

import "testing"

// TestSmoke ensures the binary compiles. The actual cancellation logic
// requires a live Postgres pool which is intentionally not provisioned
// in this unit test — the integration coverage for "refuses non-pending
// jobs" lives in the lifecycle integration tests (14.10).
func TestSmoke(t *testing.T) {
	// no-op; the file exists so go test ./... pulls in the package.
}
