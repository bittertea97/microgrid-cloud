#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

PG_IMAGE="${PG_IMAGE:-postgres:15-alpine}"
PG_USER="${PG_USER:-microgrid}"
PG_PASSWORD="${PG_PASSWORD:-microgrid}"
PG_DB="${PG_DB:-microgrid_smoke}"
PG_PORT="${PG_PORT:-54329}"
CONTAINER_NAME="${CONTAINER_NAME:-migrations-smoke-$RANDOM}"

cleanup() {
  docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }
}

require_cmd docker

echo "==> Starting Postgres container: $CONTAINER_NAME"
docker run -d --name "$CONTAINER_NAME" \
  -e POSTGRES_USER="$PG_USER" \
  -e POSTGRES_PASSWORD="$PG_PASSWORD" \
  -e POSTGRES_DB="$PG_DB" \
  -p "$PG_PORT:5432" \
  -v "$PWD/migrations:/migrations:ro" \
  "$PG_IMAGE" >/dev/null

echo "==> Waiting for Postgres"
for _ in $(seq 1 30); do
  if docker exec "$CONTAINER_NAME" pg_isready -U "$PG_USER" -d "$PG_DB" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! docker exec "$CONTAINER_NAME" pg_isready -U "$PG_USER" -d "$PG_DB" >/dev/null 2>&1; then
  echo "postgres did not become ready" >&2
  exit 1
fi

echo "==> Applying migrations"
docker exec "$CONTAINER_NAME" sh -c "for f in /migrations/*.sql; do psql -U '$PG_USER' -d '$PG_DB' -f \"\$f\"; done"

echo "==> Verifying required tables"
expected=(
  telemetry_points
  analytics_statistics
  settlements_day
  stations
  commands
  settlement_statements
  alarms
  event_outbox
  dead_letter_events
  processed_events
  shadowrun_reports
)

found="$(docker exec "$CONTAINER_NAME" psql -U "$PG_USER" -d "$PG_DB" -tAc "SELECT relname FROM pg_class WHERE relname IN ('telemetry_points','analytics_statistics','settlements_day','stations','commands','settlement_statements','alarms','event_outbox','dead_letter_events','processed_events','shadowrun_reports')")"
found="$(echo "$found" | tr -d ' ' | sed '/^$/d')"

missing=0
for table in "${expected[@]}"; do
  if ! echo "$found" | grep -qx "$table"; then
    echo "missing table: $table" >&2
    missing=1
  fi
done

if [[ "$missing" -ne 0 ]]; then
  exit 1
fi

echo "==> migrations smoke OK"
