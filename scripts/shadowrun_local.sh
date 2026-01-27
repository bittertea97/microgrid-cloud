#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

BASE_URL="${BASE_URL:-http://localhost:8080}"
PG_DSN="${PG_DSN:-${DATABASE_URL:-}}"
TENANT_ID="${TENANT_ID:-tenant-demo}"
STATION_ID="${STATION_ID:-station-demo-001}"
MONTH="${MONTH:-2026-01}"
REPORT_DIR="${REPORT_DIR:-var/reports/shadowrun}"
AUTH_JWT_SECRET="${AUTH_JWT_SECRET:-${JWT_SECRET:-}}"
JWT_ROLE="${JWT_ROLE:-admin}"
JWT_SUBJECT="${JWT_SUBJECT:-release-manager}"
JWT_TTL_SECONDS="${JWT_TTL_SECONDS:-3600}"

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1"; exit 1; }
}

require_cmd curl
require_cmd psql
require_cmd date
require_cmd openssl
if ! command -v jq >/dev/null 2>&1 && ! command -v python >/dev/null 2>&1 && ! command -v python3 >/dev/null 2>&1 && ! command -v py >/dev/null 2>&1; then
  echo "missing required command: jq or python" >&2
  exit 1
fi

if [[ -z "$PG_DSN" ]]; then
  echo "PG_DSN or DATABASE_URL must be set"
  exit 1
fi
if [[ -z "$AUTH_JWT_SECRET" ]]; then
  echo "AUTH_JWT_SECRET (or JWT_SECRET) must be set"
  exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib_auth.sh
source "$script_dir/lib_auth.sh"
# shellcheck source=lib_json.sh
source "$script_dir/lib_json.sh"

JWT_TOKEN="$(jwt_token_hs256 "$AUTH_JWT_SECRET" "$TENANT_ID" "$JWT_ROLE" "$JWT_SUBJECT" "$JWT_TTL_SECONDS")"
AUTH_HEADER="Authorization: Bearer $JWT_TOKEN"

echo "==> Apply migrations"
for f in $(ls migrations/*.sql | sort); do
  psql "$PG_DSN" -f "$f"
done

echo "==> Trigger shadowrun"
curl -sS -X POST "$BASE_URL/api/v1/shadowrun/run" \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"station_ids\": [\"$STATION_ID\"],
    \"month\": \"$MONTH\",
    \"thresholds\": { \"energy_abs\": 5, \"amount_abs\": 5, \"missing_hours\": 2 }
  }" | json_pretty

echo "==> List reports"
from="${MONTH}-01T00:00:00Z"
to="$(date -u -d \"$MONTH-01 +1 month\" +\"%Y-%m-%dT%H:%M:%SZ\")"
curl -sS -H "$AUTH_HEADER" "$BASE_URL/api/v1/shadowrun/reports?station_id=$STATION_ID&from=$from&to=$to" | json_pretty

echo "==> Done. Reports stored under $REPORT_DIR"
