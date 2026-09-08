# Backup & Restore

SQLite backup/restore for the Noumena database.

## Symptoms

- Need a restorable point-in-time copy for disaster recovery / RPO ≤ 1h.
- Restoring from "the last backup" after a deletion mistake or corruption.

## Verification

- `scripts/backup-db.sh` exits 0 and leaves a `noumena-<timestamp>.db` in
  the backup dir (default `./backups`).
- Restore is exercised on a throwaway copy before touching the live file.

## Recovery

Backup (WAL-safe online backup):

```sh
bash scripts/backup-db.sh                 # checkpoint + sqlite3 .backup + rotate
```

The script:
1. Runs `wal_checkpoint(TRUNCATE)` on the live DB.
2. Copies via `sqlite3 .backup` to `backups/noumena-$(date -u +%Y%m%dT%H%M%SZ).db`
   (safe while the app is running — no downtime).
3. Rotates to the newest `BACKUP_KEEP` copies (default 7).

Restore:

```sh
# 1. Stop the API process (single-writer; the file must not be open).
systemctl stop noumena-api        # or kill the api process

# 2. Replace the live file (preserve the WAL/SHM sidecars).
mv noumena.db noumena.db.corrupt
cp backups/noumena-2026-09-08T000000Z.db noumena.db
rm -f noumena.db-wal noumena.db-shm   # stale WAL must not replay over the restore

# 3. Start the API again.
systemctl start noumena-api
```

## Postmortem

- Note the restore timestamp and whether any writes after the backup are lost
  (RPO). SQLite is single-writer — restore only ever applies to one replica.

## Scheduled backup (optional)

Systemd timer (or cron) — docs only, no timer shipped:

```ini
# /etc/systemd/system/noumena-backup.service
[Service]
Type=oneshot
WorkingDirectory=/srv/noumena
ExecStart=/srv/noumena/scripts/backup-db.sh
```

```ini
# /etc/systemd/system/noumena-backup.timer
[Timer]
OnCalendar=*-*-* 02:00:00
Persistent=true
```

Equivalent cron: `0 2 * * * /srv/noumena/scripts/backup-db.sh`.
