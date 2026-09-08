# Incident Response

Basic incident handling for the Noumena platform.

## Symptoms

- API unhealthy, slow, or 5xx-heavy.
- Database locked / `database is locked` errors.
- Export dir full (see [`export-disk-monitoring.md`](export-disk-monitoring.md)).

## Verification

- `GET /health` → `{"status":"ok",...}` (liveness).
- `GET /ready` → `{"status":"ok"}` when the DB ping succeeds.
- `GET /metrics` → OTel metrics flowing.
- Check container health: `docker inspect --format '{{.State.Health.Status}}' <name>`.

## Recovery

1. **Isolate:** check `/ready`; if the DB ping fails, the process may have
   lost the SQLite file/mount.
2. **Single-writer rule:** SQLite does not support concurrent writers — ensure
   `MAX_REPLICAS=1` for the API (see `docs/deploy-coolify.md`). Multiple
   replicas sharing one SQLite file cause `database is locked`.
3. **Disk:** if `EXPORT_DIR` is full, delete expired exports
   (`EXPORT_TTL_HOURS` swept by cleanup cron) or grow the volume.
4. **Corruption / data loss:** restore from the last backup
   (see [`backup-restore.md`](backup-restore.md)).
5. **Workers:** validate feature flags (`INGESTION_WORKER_ENABLED`,
   `LLM_PROCESSOR_ENABLED`, `LIFECYCLE_WORKERS_ENABLED`) match the intended
   fleet topology.

## Postmortem

- Record start/end time, root cause, and any doc/code fix needed.
- If a runbook was missing a step, update it.
