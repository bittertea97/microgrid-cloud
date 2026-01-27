#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

BASE_URL="${BASE_URL:-http://localhost:8080}"
PG_DSN="${PG_DSN:-${DATABASE_URL:-}}"
AUTH_JWT_SECRET="${AUTH_JWT_SECRET:-${JWT_SECRET:-}}"
INGEST_HMAC_SECRET="${INGEST_HMAC_SECRET:-}"
AUTO_START_DOCKER="${AUTO_START_DOCKER:-1}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.dev.yml}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }
}

require_cmd bash
require_cmd curl
require_cmd openssl
require_cmd date
if ! command -v jq >/dev/null 2>&1 && ! command -v python >/dev/null 2>&1 && ! command -v python3 >/dev/null 2>&1 && ! command -v py >/dev/null 2>&1; then
  echo "missing required command: jq or python" >&2
  exit 1
fi

if [[ -z "$AUTH_JWT_SECRET" ]]; then
  AUTH_JWT_SECRET="dev-secret-change-me"
fi
if [[ -z "$INGEST_HMAC_SECRET" ]]; then
  INGEST_HMAC_SECRET="dev-ingest-secret"
fi

if [[ -z "$PG_DSN" && "$AUTO_START_DOCKER" == "1" && -x "$(command -v docker)" ]]; then
  PG_DSN="postgres://microgrid:microgrid@localhost:5432/microgrid?sslmode=disable"
fi

if [[ "$AUTO_START_DOCKER" == "1" && -x "$(command -v docker)" ]]; then
  echo "==> Starting docker compose stack"
  docker compose -f "$COMPOSE_FILE" up -d
fi

if [[ -n "$PG_DSN" && -x "$(command -v psql)" ]]; then
  echo "==> Applying migrations"
  for f in $(ls migrations/*.sql | sort); do
    psql "$PG_DSN" -f "$f"
  done
else
  echo "==> Skipping migrations (PG_DSN or psql missing)"
fi

if command -v promtool >/dev/null 2>&1; then
  echo "==> Validating Prometheus rules"
  promtool check rules alerts/*.yml
fi

if ! curl -sSf "$BASE_URL/healthz" >/dev/null; then
  echo "==> Waiting for service to become healthy"
  for _ in $(seq 1 30); do
    if curl -sSf "$BASE_URL/healthz" >/dev/null; then
      break
    fi
    sleep 2
  done
fi

if ! curl -sSf "$BASE_URL/healthz" >/dev/null; then
  echo "service health check failed: $BASE_URL/healthz" >&2
  exit 1
fi

APPLY_MIGRATIONS=0
if [[ -n "$PG_DSN" && -x "$(command -v psql)" ]]; then
  APPLY_MIGRATIONS=1
fi

set +e

echo "==> Running pilot E2E"
BASE_URL="$BASE_URL" PG_DSN="$PG_DSN" AUTH_JWT_SECRET="$AUTH_JWT_SECRET" INGEST_HMAC_SECRET="$INGEST_HMAC_SECRET" APPLY_MIGRATIONS=$APPLY_MIGRATIONS bash scripts/pilot_e2e.sh
pilot_status=$?

if [[ $pilot_status -ne 0 ]]; then
  echo "pilot E2E failed (exit=$pilot_status)" >&2
  exit $pilot_status
fi

echo "==> Running shadowrun E2E"
BASE_URL="$BASE_URL" PG_DSN="$PG_DSN" AUTH_JWT_SECRET="$AUTH_JWT_SECRET" bash scripts/shadowrun_local.sh
shadow_status=$?

if [[ $shadow_status -ne 0 ]]; then
  echo "shadowrun E2E failed (exit=$shadow_status)" >&2
  exit $shadow_status
fi

set -e

echo "==> Release acceptance complete"
