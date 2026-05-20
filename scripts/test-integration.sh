#!/usr/bin/env bash
#
# scripts/test-integration.sh — run the integration test suite.
#
# This script runs all Go tests built with the `integration` build tag.
# Tests under that tag depend on the dbtest harness in
# `internal/dbtest/`, which boots a real Postgres 16 instance via
# testcontainers-go (or an external server pointed at by
# TEST_DATABASE_URL).
#
# Usage:
#   scripts/test-integration.sh [extra `go test` args]
#
# Environment:
#   TEST_DATABASE_URL — optional Postgres DSN. If set, the harness uses
#                       that server instead of starting a container; the
#                       role must be able to CREATE DATABASE.
#
# CI invokes this script directly. Developers should prefer
# `make test-integration`.

set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> Running integration tests (-tags=integration)"
exec go test -tags=integration -race -timeout=10m "$@" ./...
