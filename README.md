# Noumena

Backend API for Noumena. See `openspec/` for specifications and active changes.

## Operations

### Cancel a pending account deletion

When a user contacts support within the 30-day grace window after requesting
account deletion, ops can reverse the request with the `cancel_deletion` CLI:

```sh
# Dry-run (validates state, prints what would change, modifies nothing):
go run ./tools/cancel_deletion --email user@example.com

# Execute:
go run ./tools/cancel_deletion --email user@example.com --confirm
```

The tool will:

1. Look up the user by email under a `FOR UPDATE` lock.
2. Refuse unless `account_deletion_jobs.status = 'pending'` — once the
   deletion worker has started (`processing`), cancellation is unsafe
   because OAuth tokens may already have been revoked or comments deleted.
3. Atomically clear `users.deletion_requested_at` and delete the
   `account_deletion_jobs` row in a single transaction.

`DATABASE_URL` is read from the environment by default; pass `--dsn` to
override.

### Lifecycle worker flags

- `LIFECYCLE_WORKERS_ENABLED=true` — start deletion + export + cleanup
  workers in this process. Production typically runs these on a single
  replica while other API replicas leave the flag off.
- `ACCOUNT_LIFECYCLE_API_ENABLED=true` — expose the
  `/api/user/account*`, `/api/user/data-export*`, and
  `/api/exports/{id}/download` endpoints. Independent of the worker
  flag so the schema + backend can ship before the user-facing surface.

### Email pipeline flags

- `EMAIL_ENABLED=true` — send via the SMTP pipeline (uses
  `email_send_log` for idempotency + retry). When `false`, the legacy
  Resend client (or mock) is used.
- The retry worker drains failed sends with exponential backoff (1m / 1h
  / 6h, 3-attempt cap) and logs each attempt.

## Configuration

All runtime configuration is driven by environment variables. The
authoritative reference is **[`.env.example`](./.env.example)** — copy
it to `.env` for local dev or use as a template for your production
secret manager. Every variable read by `internal/config/config.go` is
listed there with a `# REQUIRED | optional` annotation, grouped by
feature area (auth, ingestion, LLM, billing, email, lifecycle, OTel).
The pairing is enforced by `go test ./internal/config/...`
(`TestEnvExampleParity`), so the list cannot drift.

### Running with Docker

The image runs as a non-root user and requires at minimum:

- `ENCRYPTION_KEY` — raw AES key, 16/24/32 bytes (fail-fast).
- `JWT_SECRET` — ≥32 bytes (fail-fast).
- `DATABASE_PATH` — point at a persistent volume, e.g. `/data/noumena.db`
  (the container pre-creates `/data` owned by the runtime user).

`EXPORT_DIR` (default `/var/lib/noumena/exports`) and
`EXPORT_STORAGE_DIR` (default `/var/lib/noumena/data-exports`) are
pre-created and writable by the non-root user. Health is exposed at
`/healthz` (and `/health`) and wired to the image `HEALTHCHECK`.

> **How to add an env var.** When you introduce a new `os.Getenv(…)`
> call in `internal/config/config.go`, add the variable to
> `.env.example` in the same PR. The matching pair is enforced by code
> review, not CI — keep it honest.

### Rollback semantics

Both lifecycle flags are independently togglable at runtime restart:

- Setting `LIFECYCLE_WORKERS_ENABLED=false` halts deletion + export
  processing; jobs already enqueued will sit in `pending` until a
  replica is restarted with the flag on. No data loss; the API surface
  (if `ACCOUNT_LIFECYCLE_API_ENABLED=true`) continues to accept new
  requests.
- Setting `ACCOUNT_LIFECYCLE_API_ENABLED=false` removes the routes
  immediately (5xx → 404 from chi). Existing in-flight signed download
  URLs become unreachable; users may need to request a new export.
- Setting `EMAIL_ENABLED=false` reverts to the Resend (or mock) client;
  the `email_send_log` stops growing but pending rows from prior
  runs remain claimable when the flag is re-enabled.

## Runbooks

### "User requested deletion in error"

Use `tools/cancel_deletion` (above). Eligibility window:
`account_deletion_jobs.status='pending'` AND
`users.deletion_requested_at + DELETION_REVERSIBLE_HOURS > NOW()`.
Past either gate, recovery requires restoring from the last backup.

### "GDPR export missing"

1. Look up the export job: `SELECT * FROM data_export_jobs WHERE user_id=$1 ORDER BY requested_at DESC LIMIT 1;`
2. If `status='failed'`, inspect `last_error` and the worker logs.
   Re-enqueue by setting `status='pending'`, `attempt_count=0` and
   clearing `started_at`.
3. If `status='completed'` but `users.data_export_url` is NULL, the
   cleanup cron has already swept the file (TTL elapsed). Re-enqueue
   with the same procedure to regenerate.
4. If `status='completed'` and the file exists but the user reports a
   404, verify the signature parameters match the persisted
   `storage_path` and that `expires_at > NOW()`.

### "Stuck `email_send_log` rows"

Symptoms: `SELECT count(*) FROM email_send_log WHERE status='pending'
AND attempt_count >= 3` returns > 0.

1. Inspect `last_error` on a sample row.
2. If the error indicates SMTP credentials, validate `SMTP_USERNAME`,
   `SMTP_PASSWORD`, and DNS records (SPF/DKIM/DMARC) at the configured
   provider.
3. To replay manually after a credentials fix:
   ```sql
   UPDATE email_send_log SET status='pending', attempt_count=0,
     last_error=NULL, last_attempted_at=NULL
    WHERE status='failed' AND template='email_verification'
      AND created_at > NOW() - INTERVAL '24 hours';
   ```
   The retry worker will pick them up on the next 60-second tick.

### SMTP provider setup

The pipeline expects SPF, DKIM, and DMARC records configured at the
chosen provider (Postmark, SES, etc.) for `EMAIL_FROM_ADDRESS`. Without
these records the deliverability of transactional emails will degrade
sharply. The provider's documentation will list the required DNS
entries; verify with `dig TXT _dmarc.<domain>` after publication.

## Documentation

Operator and integrator documentation lives under [`docs/`](docs/).
Start at [`docs/README.md`](docs/README.md) for the curated index.

- API reference: [`docs/api/`](docs/api/) — auth, channels,
  comments, insights, billing, user.
- Operations runbooks: [`docs/ops/runbooks/`](docs/ops/runbooks/)
  — export disk, ingestion deployment, weekly digest, YouTube
  OAuth setup, account deletion / GDPR, channel grace window.
- End-user help: [`docs/help/`](docs/help/) — backfill window,
  CSV export schema.

Capability specs under [`specs/`](specs/) remain the source of
truth for behaviour; documentation is a navigation layer over
them.
