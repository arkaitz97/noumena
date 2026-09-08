#!/usr/bin/env bash
#
# scripts/test-integration.sh — run the DB-backed test suite.
#
# This script runs the DB-backed Go tests (internal/db + internal/dbtest)
# against the dbtest harness (in-memory SQLite, real ApplyMigrations path —
# no Docker, no external services). Note: the legacy `-tags=integration`
# suite (channel/ingestion smoke tests) still carries pre-migration
# pgxpool-era code and is deferred — see specs/DEFERRED_TASKS.md.
#
# Usage:
#   scripts/test-integration.sh [extra `go test` args]
#
# CI invokes this script directly. Developers should prefer
# `make test-integration`.

set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> Running DB-backed tests (SQLite)"
exec go test -race -timeout=10m "$@" ./internal/db/... ./internal/dbtest/...
