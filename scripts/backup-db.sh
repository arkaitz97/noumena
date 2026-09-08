#!/usr/bin/env bash
#
# scripts/backup-db.sh — SQLite online backup with WAL checkpoint + rotation.
#
# Safe to run while the API is live: PRAGMA wal_checkpoint(TRUNCATE) flushes
# the WAL, then sqlite3 .backup produces a consistent snapshot without
# stopping the writer.
#
# Usage:
#   bash scripts/backup-db.sh
#
# Environment:
#   DATABASE_PATH — SQLite file to back up (default ./noumena.db)
#   BACKUP_DIR    — destination directory (default ./backups)
#   BACKUP_KEEP   — number of backups to keep after rotation (default 7)
#
# Exits non-zero if sqlite3 is missing or the backup fails.
set -euo pipefail

DB="${DATABASE_PATH:-./noumena.db}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
BACKUP_KEEP="${BACKUP_KEEP:-7}"

if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "backup-db: sqlite3 CLI not found; install it (e.g. apk add sqlite / apt install sqlite3)" >&2
    exit 1
fi
if [ ! -f "$DB" ]; then
    echo "backup-db: database file not found: $DB" >&2
    exit 1
fi

mkdir -p "$BACKUP_DIR"

echo "==> Checkpointing WAL on $DB"
sqlite3 "$DB" "PRAGMA wal_checkpoint(TRUNCATE);"

TS="$(date -u +%Y%m%dT%H%M%SZ)"
DEST="${BACKUP_DIR}/noumena-${TS}.db"

echo "==> Backing up to $DEST"
sqlite3 "$DB" ".backup '${DEST}'"

echo "==> Verifying backup integrity"
sqlite3 "$DEST" "PRAGMA integrity_check;" | grep -q '^ok$'

echo "==> Rotating (keeping $BACKUP_KEEP)"
ls -1t "${BACKUP_DIR}"/noumena-*.db 2>/dev/null | tail -n +$((BACKUP_KEEP + 1)) | while read -r old; do
    rm -f "$old"
    echo "    removed $old"
done

echo "==> Backup complete: $DEST"