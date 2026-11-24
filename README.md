# Noumena

Growth intelligence for content creators. Noumena ingests the comment stream
from a creator's connected channels, classifies it with LLMs, and returns
structured insight — recurring questions, audience pain points, content gaps,
collaboration hooks and product-validation signals — instead of a wall of
noise.

Personal project, in active development. Not a hosted service.

## What it does

- Connects channels through OAuth (YouTube today).
- Ingests comments on a schedule, with retries and idempotent backfill.
- Classifies and clusters comments with an LLM pipeline, with failover across
  providers.
- Produces per-channel insight reports and exports.
- Bills metered usage through Stripe, including subscription lifecycle and
  overage handling.
- Handles account lifecycle: verification, password reset, 30-day deletion
  grace window with a reversal CLI.

## Stack

| Layer | Choice |
|---|---|
| Backend | Go 1.25, standard library HTTP, SQLite |
| Frontend | Vue 3, Vite, TypeScript |
| Auth | JWT access/refresh, Google SSO, AES-GCM encrypted provider tokens |
| Billing | Stripe subscriptions + metered overage |
| LLM | Provider-agnostic client with failover |
| Observability | Structured logs, request IDs, telemetry hooks |

## Layout

```
cmd/api            HTTP entrypoint
internal/api       routing, handlers, middleware
internal/auth      JWT, sessions, password policy
internal/oauth     Google and YouTube OAuth flows
internal/ingestion scheduled comment ingestion, workers, backfill
internal/llm       classification pipeline, provider clients, failover
internal/billing   Stripe subscriptions, metering, overage
internal/lifecycle account deletion grace window and sweeper
internal/db        schema, migrations, queries
internal/dbtest    database test harness
internal/telemetry logs, metrics, tracing hooks
tools              operational CLIs (seeding, deletion reversal)
openspec           specifications and change proposals
```

Design notes and product requirements live in `PRD.md` and `openspec/`.

## Run it

```sh
cp .env.example .env        # fill in the OAuth, Stripe and LLM credentials
make seed                   # create a local database with sample data
make dev                    # backend on :8080, frontend via Vite
```

`make stop` shuts both dev servers down.

## Tests

```sh
make test               # fast suite, no external services
make test-integration   # database-backed suite, race detector
make fmt-check          # gofmt gate
```

88 test files cover ingestion, classification, billing, auth and lifecycle.

## Notes

- The repository contains no credentials. `.env.example` lists the keys the
  application expects.
- Some operational tooling assumes a local SQLite database; there is no
  production deployment configuration in this repository.
