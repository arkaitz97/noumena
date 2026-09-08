# Ingestion Deployment

Phased feature-flag rollout for the comment-ingestion fleet.

## Symptoms

- Rolling out the ingestion worker without a clear flag order.
- `database is locked` when more than one replica ingests.

## Verification

- Single-writer constraint: SQLite supports one writer — keep
  `INGESTION_WORKER_ENABLED=true` on exactly one replica (see
  `docs/deploy-coolify.md` `MAX_REPLICAS=1`).

## Rollout

1. Deploy with `INGESTION_WORKER_ENABLED=false` — API serves normally,
   no jobs consumed.
2. Enable the worker on one instance:
   `INGESTION_WORKER_ENABLED=true`.
3. Run the initial-backfill enqueue script:
   `DATABASE_PATH=./noumena.db go run ./scripts/enqueue_initial_backfills`.
4. Configure dead-letter alerting destination in production
   (`INGESTION_DEAD_LETTER_ALERT_TO`).

Workers claim jobs atomically (`UPDATE … WHERE status='pending' … RETURNING`).
Dead-lettered jobs park the channel for 1 hour; retry budget is
`INGESTION_MAX_RETRIES` (default 5).

## Postmortem

- If jobs dead-letter en masse, check YouTube quota / token validity first.
