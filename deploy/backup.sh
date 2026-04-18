#!/usr/bin/env bash
# Daily backup for the SEO system SQLite DB.
#
# Install (root):
#   cp deploy/backup.sh /usr/local/bin/seo-system-backup.sh
#   chmod +x /usr/local/bin/seo-system-backup.sh
#   cp deploy/seo-system-backup.timer /etc/systemd/system/
#   cp deploy/seo-system-backup.service /etc/systemd/system/
#   systemctl daemon-reload
#   systemctl enable --now seo-system-backup.timer
#
# Env (override in systemd drop-in if needed):
#   DB_PATH       — source DB. Default /opt/seo-system/data/seo.db
#   BACKUP_DIR    — destination. Default /var/backups/seo-system
#   KEEP_DAYS     — purge older files. Default 7
set -euo pipefail

DB_PATH="${DB_PATH:-/opt/seo-system/data/seo.db}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/seo-system}"
KEEP_DAYS="${KEEP_DAYS:-7}"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
dest="${BACKUP_DIR}/seo-${timestamp}.db"

mkdir -p -m 700 "${BACKUP_DIR}"

# Use sqlite's online backup so a running server doesn't corrupt the output.
sqlite3 "${DB_PATH}" ".backup '${dest}'"
gzip -9 "${dest}"
chmod 600 "${dest}.gz"

# Prune older than KEEP_DAYS.
find "${BACKUP_DIR}" -name 'seo-*.db.gz' -type f -mtime +"${KEEP_DAYS}" -print -delete

echo "[backup] wrote ${dest}.gz"
