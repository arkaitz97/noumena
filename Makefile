# Noumena — Makefile
#
# Conventions:
#   make test              # default fast suite, no Docker required
#   make test-integration  # DB-backed tests (SQLite, no Docker)
#   make dev               # start backend + frontend in watch mode
#   make stop              # stop dev servers

.PHONY: test test-integration dev stop seed

test:
	go test ./...

test-integration:
	go test ./internal/db/... ./internal/dbtest/...

dev:
	@echo "Starting dev servers..."
	@trap 'kill 0' EXIT INT TERM; \
	go run ./cmd/api & \
	cd frontend && npm run dev & \
	wait

stop:
	@pkill -f "go run ./cmd/api" 2>/dev/null || true
	@pkill -f "vite" 2>/dev/null || true
	@echo "Dev servers stopped."

seed:
	go run ./tools/seed_db
