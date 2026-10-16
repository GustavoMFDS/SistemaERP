#!/usr/bin/env bash
# Scheduled PostgreSQL logical backups for an authorized operations account.
# Configure PGHOST/PGPORT/PGDATABASE/PGUSER/PGPASSFILE (no passwords in CLI).
set -euo pipefail
umask 077

: "${BACKUP_DIR:?set absolute BACKUP_DIR outside the repository}"
: "${PGDATABASE:?set PGDATABASE}"
: "${PGHOST:?set PGHOST}"
: "${PGUSER:?set PGUSER}"

case "$BACKUP_DIR" in /*) ;; *) echo "BACKUP_DIR must be absolute" >&2; exit 2;; esac
case "$BACKUP_DIR" in /|/tmp|/var/tmp) echo "unsafe BACKUP_DIR" >&2; exit 2;; esac
retention="${BACKUP_RETENTION_DAYS:-14}"
[[ "$retention" =~ ^[0-9]+$ ]] && (( retention >= 7 && retention <= 3650 )) || {
  echo "BACKUP_RETENTION_DAYS must be 7..3650" >&2; exit 2;
}
for name in pg_dump pg_restore sha256sum date find mktemp flock; do
  command -v "$name" >/dev/null || { echo "missing $name" >&2; exit 2; }
done
mkdir -p -- "$BACKUP_DIR"
chmod 700 -- "$BACKUP_DIR"
exec 9>"$BACKUP_DIR/.esterium-backup.lock"
flock -n 9 || { echo "backup already in progress" >&2; exit 3; }

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
tmp="$(mktemp "$BACKUP_DIR/.esterium-XXXXXXXX.dump.tmp")"
finished=0
status_path="${BACKUP_STATUS_PATH:-$BACKUP_DIR/backup-status.json}"
# Write status only within BACKUP_DIR; NEVER place credentials, tenant data
# or database names in the monitoring status file.
case "$status_path" in "$BACKUP_DIR/"*) ;; *) echo "BACKUP_STATUS_PATH must be inside BACKUP_DIR" >&2; exit 2;; esac
write_status() {
  local state="$1" now tmp_status
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  tmp_status="$(mktemp "$BACKUP_DIR/.esterium-status-XXXXXXXX.tmp")"
  printf '{"last_run_utc":"%s","state":"%s","archive_verified":%s}\n' "$now" "$state" "$([ "$state" = "ok" ] && echo true || echo false)" > "$tmp_status"
  mv -- "$tmp_status" "$status_path"
}
cleanup() {
  local result="$?"
  if (( finished == 0 )); then
    rm -f -- "$tmp"
    write_status failed || true
  fi
  exit "$result"
}
trap cleanup EXIT

# Prevent false "success" on dirty/unknown migration states.
schema="$(psql -v ON_ERROR_STOP=1 -At -F '|' -c 'SELECT version, dirty FROM schema_migrations LIMIT 1')"
[[ "$schema" =~ ^[0-9]+\|f$ ]] || { echo "dirty or missing schema" >&2; exit 1; }

pg_dump --format=custom --compress=6 --no-owner --no-privileges --file="$tmp"
test -s "$tmp"
pg_restore --list "$tmp" >/dev/null
final="$BACKUP_DIR/esterium-$stamp.dump"
# Rename only AFTER validation; never overwrite existing backup.
if [ -e "$final" ]; then echo "backup filename collision" >&2; exit 1; fi
mv -- "$tmp" "$final"
(cd "$BACKUP_DIR"; sha256sum "$(basename "$final")" > "$(basename "$final").sha256"; sha256sum -c "$(basename "$final").sha256")
finished=1
write_status ok
# Delete ONLY our own verified old dumps, checksum pairs and no other files.
find "$BACKUP_DIR" -maxdepth 1 -type f -name 'esterium-*.dump' -mtime +"$retention" -print0 |
  while IFS= read -r -d '' old; do
    [ -f "$old.sha256" ] || continue
    rm -f -- "$old" "$old.sha256"
  done
trap - EXIT
printf 'PASS: logical backup verified (%s)\n' "$stamp"
