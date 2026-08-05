#!/usr/bin/env bash
#
# Nightly backup for the NiosEngine VPS stack.
#
# Backs up two things:
#   1. Postgres  — logical pg_dump (tenants, users, leads, messages).
#   2. bridge_state volume — per-session whatsmeow device state. THIS HAS NO
#      OTHER COPY. Losing it forces every tenant to re-scan a WhatsApp QR code,
#      so it matters as much as the database.
#
# Install:
#   sudo cp deploy/vps/backup.sh /usr/local/bin/niosengine-backup
#   sudo chmod +x /usr/local/bin/niosengine-backup
#   ( crontab -l 2>/dev/null; echo "17 3 * * * /usr/local/bin/niosengine-backup" ) | crontab -
#
# Restore: see deploy/vps/README.md.

set -euo pipefail

PROJECT_DIR="${PROJECT_DIR:-$HOME/niosengine}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/niosengine}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.prod.yml}"
ENV_FILE="${ENV_FILE:-.env.prod}"
PROJECT_NAME="${PROJECT_NAME:-niosengine}"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%SZ)" "$*"; }
fail() {
	printf '[%s] ERROR: %s\n' "$(date -u +%H:%M:%SZ)" "$*" >&2
	exit 1
}

cd "$PROJECT_DIR" 2>/dev/null || fail "PROJECT_DIR not found: $PROJECT_DIR"
[[ -f "$ENV_FILE" ]] || fail "env file not found: $PROJECT_DIR/$ENV_FILE"

mkdir -p "$BACKUP_DIR"

compose() { docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"; }

# shellcheck source=/dev/null
set -a && . "./$ENV_FILE" && set +a
PGUSER="${POSTGRES_USER:-niosengine}"
PGDB="${POSTGRES_DB:-niosengine}"

# ---- 1. Postgres ---------------------------------------------------------
DB_FILE="$BACKUP_DIR/pg-${STAMP}.sql.gz"
log "Dumping Postgres ($PGDB) -> $DB_FILE"
# --clean --if-exists makes the dump self-contained for a drop-in restore.
if compose exec -T postgres pg_dump \
	--username="$PGUSER" \
	--dbname="$PGDB" \
	--clean --if-exists --no-owner --no-privileges |
	gzip -9 >"$DB_FILE"; then
	# A dump of a running-but-broken DB can be near empty; catch that here
	# rather than discovering it during a restore.
	size=$(wc -c <"$DB_FILE")
	[[ "$size" -gt 1024 ]] || fail "dump suspiciously small (${size} bytes) — not pruning old backups"
	log "Postgres dump OK (${size} bytes)"
else
	rm -f "$DB_FILE"
	fail "pg_dump failed"
fi

# ---- 2. Bridge device state ---------------------------------------------
STATE_FILE="$BACKUP_DIR/bridge-state-${STAMP}.tar.gz"
VOLUME="${PROJECT_NAME}_bridge_state"
log "Archiving volume $VOLUME -> $STATE_FILE"

if ! docker volume inspect "$VOLUME" >/dev/null 2>&1; then
	fail "volume $VOLUME not found (set PROJECT_NAME if your compose project differs)"
fi

# SQLite files are being written live. Stopping the bridge for a consistent
# copy would drop WhatsApp connections, so instead copy and rely on WAL mode:
# a WAL-mode DB copied this way recovers on open. Good enough for a nightly.
docker run --rm \
	-v "$VOLUME":/state:ro \
	-v "$BACKUP_DIR":/backup \
	alpine:3 \
	tar czf "/backup/$(basename "$STATE_FILE")" -C /state . ||
	fail "bridge state archive failed"

log "Bridge state archive OK ($(wc -c <"$STATE_FILE") bytes)"

# ---- 3. Prune ------------------------------------------------------------
log "Pruning backups older than ${RETENTION_DAYS} days"
find "$BACKUP_DIR" -maxdepth 1 -name 'pg-*.sql.gz' -mtime "+${RETENTION_DAYS}" -delete
find "$BACKUP_DIR" -maxdepth 1 -name 'bridge-state-*.tar.gz' -mtime "+${RETENTION_DAYS}" -delete

log "Done. Current backups:"
ls -1sh "$BACKUP_DIR" | tail -n 20

# NOTE: these backups live on the same disk as the data they protect, which
# does not survive losing the VPS. Copy them off-box — e.g. add a restic or
# rclone push to object storage here.
