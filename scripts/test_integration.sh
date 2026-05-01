#!/bin/bash
set -e

echo "Starting test database container..."
docker compose -f docker-compose.test.yml up -d --wait

export TEST_DATABASE_URL="postgresql://testuser:testpassword@localhost:54320/noumena_test?sslmode=disable"
export OPENAI_API_KEY="" # Ensure LLM batch processor doesn't fire real requests

echo "Running Go tests with coverage..."
go test -cover -coverprofile=coverage.out ./...

echo "Displaying coverage..."
go tool cover -func=coverage.out

echo "Cleaning up..."
docker compose -f docker-compose.test.yml down -v
