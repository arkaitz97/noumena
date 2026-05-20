# Noumena documentation

Operator and integrator documentation for the Noumena backend.
This index is curated — every page below is hand-maintained and
linked from at least one capability spec or runbook.

For source-of-truth requirements, see
[`../specs/`](../specs/). For the developer onboarding entry point,
see the root [`../README.md`](../README.md).

## API reference

HTTP-level reference for each capability surface. Each page links
back to its capability spec for behavioural detail.

- [`api/auth.md`](api/auth.md) — registration, login, refresh,
  logout, and Google SSO (popup + redirect modes).
- [`api/channels.md`](api/channels.md) — connect, list, pause,
  resume, remove a tracked YouTube channel; backfill-window
  semantics.
- [`api/comments.md`](api/comments.md) — synchronous and
  asynchronous comment exports; CSV schema cross-reference.
- [`api/insights.md`](api/insights.md) — read-only insights surface,
  saved-comments star/unstar, weekly-digest read endpoints.
- [`api/billing.md`](api/billing.md) — Stripe checkout, customer
  portal, webhook handlers, plan-limit reference.
- [`api/user.md`](api/user.md) — `/api/user/me`, account deletion,
  GDPR data export, signed-URL download.

## Operations runbooks

Each runbook follows the **Symptoms → Verification → Recovery →
Postmortem** shape. Configuration values are referenced from
[`../.env.example`](../.env.example) rather than duplicated.

- [`observability.md`](observability.md) — metrics, tracing, and
  the canonical meter names.
- [`ops/runbooks/export-disk-monitoring.md`](ops/runbooks/export-disk-monitoring.md)
  — `EXPORT_DIR` disk-usage alerting and cleanup verification.
- [`ops/runbooks/ingestion-deployment.md`](ops/runbooks/ingestion-deployment.md)
  — phased feature-flag rollout for the comment-ingestion fleet.
- [`ops/runbooks/weekly-digest-delivery.md`](ops/runbooks/weekly-digest-delivery.md)
  — weekly cron cadence, idempotency, retry semantics.
- [`ops/runbooks/youtube-oauth-setup.md`](ops/runbooks/youtube-oauth-setup.md)
  — Google Cloud Console project setup, required scopes.
- [`ops/runbooks/account-deletion-and-gdpr.md`](ops/runbooks/account-deletion-and-gdpr.md)
  — support workflows; `cancel_deletion` CLI usage.
- [`ops/runbooks/channel-grace-window.md`](ops/runbooks/channel-grace-window.md)
  — shared-channel grace-window behaviour for support staff.

## End-user help

Plain-English explanations suitable for customer-facing surfaces.

- [`help/backfill-window.md`](help/backfill-window.md) — how the
  backfill window works, including why changing it does **not**
  re-fetch history.
- [`help/csv-export-schema.md`](help/csv-export-schema.md) — every
  column in the CSV export, with type and nullability.

## Conventions

- Markdown is GitHub-flavored; relative links between docs use
  POSIX paths.
- Environment variables are referenced — never redefined — from
  [`../.env.example`](../.env.example).
- Capability specs under [`../specs/`](../specs/) are the source of
  truth for behaviour; documentation here is a navigation layer
  over them.
