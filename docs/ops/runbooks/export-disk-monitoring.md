# Export Disk Monitoring

`EXPORT_DIR` / `EXPORT_STORAGE_DIR` disk usage, alerting, and cleanup.

## Symptoms

- Exports start failing with ENOSPC / "no space left on device".
- `EnsureExportDir` warns at boot that the export dir is not writable.

## Verification

```sh
df -h "$(readlink -f "${EXPORT_DIR:-/var/lib/noumena/exports}")"
find "${EXPORT_STORAGE_DIR:-/var/lib/noumena/data-exports}" -type f -printf '%T@ %p\n' | sort -n | head
```

- Exports older than `EXPORT_TTL_HOURS` (default 24) are swept by the cleanup
  cron; confirm the sweep is keeping pace with production volume.

## Recovery

1. Delete expired exports manually if the cron is behind:
   ```sh
   find /var/lib/noumena/data-exports -type f -mtime +7 -delete
   ```
2. Grow the volume, or raise `EXPORT_TTL_HOURS` and adjust alerting.
3. If the dir is unwritable, fix ownership/perms (container runs as a non-root
   user and must own the dir — see the Dockerfile `USER`/`EXPORT_DIR` setup).

## Postmortem

- Record disk growth rate and projected days-to-full.
- Adjust monitoring thresholds.
