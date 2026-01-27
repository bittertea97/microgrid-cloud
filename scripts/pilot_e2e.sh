#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

BASE_URL="${BASE_URL:-http://localhost:8080}"
PG_DSN="${PG_DSN:-${DATABASE_URL:-}}"
TENANT_ID="${TENANT_ID:-tenant-demo}"
STATION_ID="${STATION_ID:-station-demo-001}"
STATION_NAME="${STATION_NAME:-station-demo-001}"
STATION_TZ="${STATION_TZ:-UTC}"
STATION_TYPE="${STATION_TYPE:-microgrid}"
STATION_REGION="${STATION_REGION:-pilot}"
DEVICE_ID="${DEVICE_ID:-device-demo-001}"
DEVICE_NAME="${DEVICE_NAME:-device-demo-001}"
DEVICE_TYPE="${DEVICE_TYPE:-inverter}"
TB_PROFILE="${TB_PROFILE:-default}"
DEVICE_CREDENTIALS="${DEVICE_CREDENTIALS:-token-123}"
MONTH="${MONTH:-2026-01}"
BASE_DAY="${BASE_DAY:-2026-01-20}"
EXPORT_DIR="${EXPORT_DIR:-var/exports}"
AUTH_JWT_SECRET="${AUTH_JWT_SECRET:-${JWT_SECRET:-}}"
INGEST_HMAC_SECRET="${INGEST_HMAC_SECRET:-}"
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
if [[ -z "$INGEST_HMAC_SECRET" ]]; then
  echo "INGEST_HMAC_SECRET must be set"
  exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib_auth.sh
source "$script_dir/lib_auth.sh"
# shellcheck source=lib_json.sh
source "$script_dir/lib_json.sh"

JWT_TOKEN="$(jwt_token_hs256 "$AUTH_JWT_SECRET" "$TENANT_ID" "$JWT_ROLE" "$JWT_SUBJECT" "$JWT_TTL_SECONDS")"
AUTH_HEADER="Authorization: Bearer $JWT_TOKEN"

ingest_post() {
  local body="$1"
  local ts sig
  ts=$(date +%s)
  sig=$(ingest_signature "$INGEST_HMAC_SECRET" "$ts" "$body")
  curl -sS -X POST "$BASE_URL/ingest/thingsboard/telemetry" \
    -H "Content-Type: application/json" \
    -H "X-Ingest-Timestamp: $ts" \
    -H "X-Ingest-Signature: $sig" \
    -d "$body"
}

echo "==> Checking service health"
curl -sSf "$BASE_URL/healthz" >/dev/null

if [[ "${APPLY_MIGRATIONS:-}" == "1" ]]; then
  echo "==> Applying migrations"
  for f in $(ls migrations/*.sql | sort); do
    psql "$PG_DSN" -f "$f"
  done
fi

echo "==> Provisioning station/device/mappings"
provision_payload=$(cat <<JSON
{
  "station": {
    "id": "$STATION_ID",
    "tenant_id": "$TENANT_ID",
    "name": "$STATION_NAME",
    "timezone": "$STATION_TZ",
    "type": "$STATION_TYPE",
    "region": "$STATION_REGION"
  },
  "devices": [
    {
      "id": "$DEVICE_ID",
      "name": "$DEVICE_NAME",
      "device_type": "$DEVICE_TYPE",
      "tb_profile": "$TB_PROFILE",
      "credentials": "$DEVICE_CREDENTIALS"
    }
  ],
  "point_mappings": [
    { "device_id": "$DEVICE_ID", "point_key": "charge_power_kw", "semantic": "charge_power_kw", "unit": "kW", "factor": 1 },
    { "device_id": "$DEVICE_ID", "point_key": "discharge_power_kw", "semantic": "discharge_power_kw", "unit": "kW", "factor": 1 },
    { "device_id": "$DEVICE_ID", "point_key": "earnings", "semantic": "earnings", "unit": "CNY", "factor": 1 },
    { "device_id": "$DEVICE_ID", "point_key": "carbon_reduction", "semantic": "carbon_reduction", "unit": "kg", "factor": 1 }
  ]
}
JSON
)
provision_resp=$(curl -sS -X POST "$BASE_URL/api/v1/provisioning/stations" \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d "$provision_payload")
echo "$provision_resp" | json_pretty

echo "==> Verify masterdata"
psql "$PG_DSN" -c "SELECT id, tenant_id, tb_asset_id FROM stations WHERE id='$STATION_ID';"
psql "$PG_DSN" -c "SELECT id, station_id, tb_entity_id FROM devices WHERE id='$DEVICE_ID';"
psql "$PG_DSN" -c "SELECT station_id, point_key, semantic, unit, factor FROM point_mappings WHERE station_id='$STATION_ID';"

echo "==> Ingest telemetry + window close (3 days)"
for day in 0 1 2; do
  for hour in $(seq 0 23); do
    ts=$(date -u -d "$BASE_DAY +$day day +$hour hour +5 min" +"%s000")
    window_start=$(date -u -d "$BASE_DAY +$day day +$hour hour" +"%Y-%m-%dT%H:00:00Z")
    ingest_body="{
      \"tenantId\": \"$TENANT_ID\",
      \"stationId\": \"$STATION_ID\",
      \"deviceId\": \"$DEVICE_ID\",
      \"ts\": $ts,
      \"values\": {
        \"charge_power_kw\": 1,
        \"discharge_power_kw\": 2,
        \"earnings\": 0.1,
        \"carbon_reduction\": 0.01
      }
    }"
    ingest_post "$ingest_body" >/dev/null
    curl -sS -X POST "$BASE_URL/analytics/window-close" \
      -H "Content-Type: application/json" \
      -H "$AUTH_HEADER" \
      -d "{
        \"stationId\": \"$STATION_ID\",
        \"windowStart\": \"$window_start\"
      }" >/dev/null
  done
done

from_ts=$(date -u -d "$BASE_DAY" +"%Y-%m-%dT00:00:00Z")
to_ts=$(date -u -d "$BASE_DAY +3 day" +"%Y-%m-%dT00:00:00Z")

echo "==> Verify analytics statistics"
psql "$PG_DSN" -c "
SELECT time_type, period_start, charge_kwh, discharge_kwh, is_completed
FROM analytics_statistics
WHERE subject_id='$STATION_ID' AND time_type IN ('HOUR','DAY')
ORDER BY period_start ASC;"
curl -sS -H "$AUTH_HEADER" "$BASE_URL/api/v1/stats?station_id=$STATION_ID&from=$from_ts&to=$to_ts&granularity=day" | json_pretty

echo "==> Verify settlements_day"
psql "$PG_DSN" -c "
SELECT day_start, energy_kwh, amount, currency, status, version
FROM settlements_day
WHERE tenant_id='$TENANT_ID' AND station_id='$STATION_ID'
ORDER BY day_start ASC;"
curl -sS -H "$AUTH_HEADER" "$BASE_URL/api/v1/settlements?station_id=$STATION_ID&from=$from_ts&to=$to_ts" | json_pretty

echo "==> Generate statement (draft) and freeze"
stmt_resp=$(curl -sS -X POST "$BASE_URL/api/v1/statements/generate" \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d "{ \"station_id\": \"$STATION_ID\", \"month\": \"$MONTH\", \"category\": \"owner\", \"regenerate\": false }")
stmt_id=$(echo "$stmt_resp" | json_get '.statement_id')
echo "$stmt_resp" | json_pretty

freeze_resp=$(curl -sS -X POST "$BASE_URL/api/v1/statements/$stmt_id/freeze" -H "$AUTH_HEADER")
echo "$freeze_resp" | json_pretty

mkdir -p "$EXPORT_DIR"
curl -sS -H "$AUTH_HEADER" -o "$EXPORT_DIR/statement.pdf" "$BASE_URL/api/v1/statements/$stmt_id/export.pdf"
curl -sS -H "$AUTH_HEADER" -o "$EXPORT_DIR/statement.xlsx" "$BASE_URL/api/v1/statements/$stmt_id/export.xlsx"
test -s "$EXPORT_DIR/statement.pdf"
test -s "$EXPORT_DIR/statement.xlsx"
echo "Exported to $EXPORT_DIR/statement.pdf and $EXPORT_DIR/statement.xlsx"

echo "==> Create alarm rule"
psql "$PG_DSN" -c "
DELETE FROM alarm_rules WHERE id='rule-pilot-001';
INSERT INTO alarm_rules (
  id, tenant_id, station_id, name, semantic, operator, threshold,
  hysteresis, duration_seconds, severity, enabled
) VALUES (
  'rule-pilot-001',
  '$TENANT_ID',
  '$STATION_ID',
  'Charge Power High',
  'charge_power_kw',
  '>',
  100,
  5,
  0,
  'high',
  TRUE
);"

echo "==> Start SSE stream (background)"
sse_log="$EXPORT_DIR/alarms_sse.log"
: > "$sse_log"
curl -N -H "$AUTH_HEADER" "$BASE_URL/api/v1/alarms/stream" >"$sse_log" 2>/dev/null &
sse_pid=$!

echo "==> Trigger alarm (active -> clear)"
ts_now=$(date -u +"%s000")
ingest_post "{
  \"tenantId\": \"$TENANT_ID\",
  \"stationId\": \"$STATION_ID\",
  \"deviceId\": \"$DEVICE_ID\",
  \"ts\": $ts_now,
  \"values\": { \"charge_power_kw\": 120 }
}" >/dev/null
sleep 2
ts_now=$(date -u +"%s000")
ingest_post "{
  \"tenantId\": \"$TENANT_ID\",
  \"stationId\": \"$STATION_ID\",
  \"deviceId\": \"$DEVICE_ID\",
  \"ts\": $ts_now,
  \"values\": { \"charge_power_kw\": 90 }
}" >/dev/null
sleep 2
kill "$sse_pid" >/dev/null 2>&1 || true

grep -q '"type":"active"' "$sse_log" && echo "SSE active OK"
grep -q '"type":"cleared"' "$sse_log" && echo "SSE cleared OK"

echo "==> Verify alarms via API"
from_alarm=$(date -u -d "-1 hour" +"%Y-%m-%dT%H:%M:%SZ")
to_alarm=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
curl -sS -H "$AUTH_HEADER" "$BASE_URL/api/v1/alarms?station_id=$STATION_ID&from=$from_alarm&to=$to_alarm" | json_pretty

echo "==> Issue command"
cmd_resp=$(curl -sS -X POST "$BASE_URL/api/v1/commands" \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d "{
    \"tenant_id\": \"$TENANT_ID\",
    \"station_id\": \"$STATION_ID\",
    \"device_id\": \"$DEVICE_ID\",
    \"command_type\": \"setPower\",
    \"payload\": { \"value\": 10 },
    \"idempotency_key\": \"setPower-20260101-001\"
  }")
cmd_id=$(echo "$cmd_resp" | json_get '.command_id')
echo "$cmd_resp" | json_pretty

sleep 2
from_cmd=$(date -u -d "-5 minutes" +"%Y-%m-%dT%H:%M:%SZ")
to_cmd=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
cmd_list=$(curl -sS -H "$AUTH_HEADER" "$BASE_URL/api/v1/commands?station_id=$STATION_ID&from=$from_cmd&to=$to_cmd")
echo "$cmd_list" | json_pretty
cmd_status=$(echo "$cmd_list" | json_get '[-1].status')
if [[ "$cmd_status" != "acked" ]]; then
  echo "Command not acked yet (status=$cmd_status). Marking timeout for pilot."
  psql "$PG_DSN" -c "UPDATE commands SET status='timeout', error='timeout' WHERE command_id='$cmd_id' AND status='sent';"
  cmd_list=$(curl -sS -H "$AUTH_HEADER" "$BASE_URL/api/v1/commands?station_id=$STATION_ID&from=$from_cmd&to=$to_cmd")
  echo "$cmd_list" | json_pretty
fi

echo "==> Backfill one hour and recalc"
frozen_total=$(curl -sS -H "$AUTH_HEADER" "$BASE_URL/api/v1/statements/$stmt_id" | json_get '.statement.total_amount')
backfill_ts=$(date -u -d "2026-01-21 06:05:00" +"%s000")
backfill_window="2026-01-21T06:00:00Z"
ingest_post "{
  \"tenantId\": \"$TENANT_ID\",
  \"stationId\": \"$STATION_ID\",
  \"deviceId\": \"$DEVICE_ID\",
  \"ts\": $backfill_ts,
  \"values\": { \"charge_power_kw\": 10, \"discharge_power_kw\": 20 }
}" >/dev/null
curl -sS -X POST "$BASE_URL/analytics/window-close" \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d "{ \"stationId\": \"$STATION_ID\", \"windowStart\": \"$backfill_window\", \"recalculate\": true }" >/dev/null

echo "==> Verify frozen statement unchanged"
frozen_after=$(curl -sS -H "$AUTH_HEADER" "$BASE_URL/api/v1/statements/$stmt_id" | json_get '.statement.total_amount')
if [[ "$frozen_total" != "$frozen_after" ]]; then
  echo "Frozen statement changed unexpectedly: $frozen_total -> $frozen_after"
  exit 1
fi

echo "==> Regenerate statement for corrected version"
regen_resp=$(curl -sS -X POST "$BASE_URL/api/v1/statements/generate" \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d "{ \"station_id\": \"$STATION_ID\", \"month\": \"$MONTH\", \"category\": \"owner\", \"regenerate\": true }")
echo "$regen_resp" | json_pretty

echo "==> Pilot E2E completed"
