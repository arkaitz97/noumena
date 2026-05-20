# Noumena — Makefile
#
# Conventions:
#   make test              # default fast suite, no Docker required
#   make test-integration  # full suite incl. DB-backed tests (Docker)

.PHONY: test test-integration

test:
	go test ./...

test-integration:
	./scripts/test-integration.sh
