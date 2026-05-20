#!/usr/bin/env bash
# Run the frontend test suite. Intended as the CI entrypoint and the
# canonical local command for "what CI runs".
#
# Usage: bash scripts/test-frontend.sh
#
# Behaviour:
#   1. cd into frontend/
#   2. npm ci   — clean, lockfile-faithful install
#   3. npm test — vitest run (one-shot, non-watch)
#
# Exits non-zero if any step fails.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}/frontend"
npm ci
npm test
