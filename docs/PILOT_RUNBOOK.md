# Pilot Runbook (End-to-End)

This runbook guides a full pilot flow from provisioning to analytics/settlement/statement, alarms, commands, and backfill validation.

All time inputs/outputs use RFC3339 UTC.

## 0) Prerequisites

Tools:
- `psql`, `curl`, `jq` or `python`
- GNU `date` (Linux). On macOS, use `gdate` from coreutils.

Environment:
```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/microgrid?sslmode=disable"
export PG_DSN="$DATABASE_URL"
export TB_BASE_URL="http://localhost:18080"   # Fake TB server in compose (use real TB base URL if needed)
export TB_TOKEN=""
export TENANT_ID="tenant-demo"
export AUTH_JWT_SECRET="dev-secret-change-me"
export INGEST_HMAC_SECRET="dev-ingest-secret"
```

Start Postgres (local):
```bash
docker run --name microgrid-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 -d postgres:15
psql "postgres://postgres:postgres@localhost:5432/postgres" -c "CREATE DATABASE microgrid;"
```

Apply migrations:
```bash
for f in $(ls migrations/*.sql | sort); do
  psql "$PG_DSN" -f "$f"
done
```

Auth setup (for API calls):
```bash
source scripts/lib_auth.sh
AUTH_HEADER="Authorization: Bearer $(jwt_token_hs256 "$AUTH_JWT_SECRET" tenant-demo admin runbook-user 3600)"
```

Signed ingest helper:
```bash
ingest_post() {
  local body="$1"
  local ts sig
  ts=$(date +%s)
  sig=$(ingest_signature "$INGEST_HMAC_SECRET" "$ts" "$body")
  curl -sS -X POST http://localhost:8080/ingest/thingsboard/telemetry \
    -H "Content-Type: application/json" \
    -H "X-Ingest-Timestamp: $ts" \
    -H "X-Ingest-Signature: $sig" \
    -d "$body"
}
```

Run service:
```bash
export HTTP_ADDR=":8080"
export DATABASE_URL="$PG_DSN"
export TENANT_ID="tenant-demo"
export STATION_ID="station-demo-001"
export PRICE_PER_KWH="1.0"
export CURRENCY="CNY"
export EXPECTED_HOURS="24"
export TB_BASE_URL="$TB_BASE_URL"
export TB_TOKEN="$TB_TOKEN"
go run .
```

## 1) Provisioning: create station/device/mappings

```bash
curl -sS -X POST http://localhost:8080/api/v1/provisioning/stations \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{
    "station": {
      "id": "station-demo-001",
      "tenant_id": "tenant-demo",
      "name": "station-demo-001",
      "timezone": "UTC",
      "type": "microgrid",
      "region": "pilot"
    },
    "devices": [
      {
        "id": "device-demo-001",
        "name": "device-demo-001",
        "device_type": "inverter",
        "tb_profile": "default",
        "credentials": "token-123"
      }
    ],
    "point_mappings": [
      { "device_id": "device-demo-001", "point_key": "charge_power_kw", "semantic": "charge_power_kw", "unit": "kW", "factor": 1 },
      { "device_id": "device-demo-001", "point_key": "discharge_power_kw", "semantic": "discharge_power_kw", "unit": "kW", "factor": 1 },
      { "device_id": "device-demo-001", "point_key": "earnings", "semantic": "earnings", "unit": "CNY", "factor": 1 },
      { "device_id": "device-demo-001", "point_key": "carbon_reduction", "semantic": "carbon_reduction", "unit": "kg", "factor": 1 }
    ]
  }'
```

Verify in DB:
```bash
psql "$PG_DSN" -c "SELECT id, tenant_id, tb_asset_id FROM stations WHERE id='station-demo-001';"
psql "$PG_DSN" -c "SELECT id, station_id, tb_entity_id FROM devices WHERE id='device-demo-001';"
psql "$PG_DSN" -c "SELECT station_id, point_key, semantic, unit, factor FROM point_mappings WHERE station_id='station-demo-001';"
```

## 2) TB telemetry forwarding (real) + curl simulation

Minimal TB setup:
- Create (or use) a rule chain that forwards telemetry to the platform ingest endpoint:
  - URL: `http://<platform-host>:8080/ingest/thingsboard/telemetry`
  - Method: `POST`
  - Body (JSON) template:
```json
{
  "tenantId": "${tenantId}",
  "stationId": "${assetName}",
  "deviceId": "${deviceName}",
  "ts": ${ts},
  "values": ${telemetry}
}
```
Use station/device naming to match your provisioning inputs.

Curl simulation (works without TB):
```bash
ingest_post '{
  "tenantId": "tenant-demo",
  "stationId": "station-demo-001",
  "deviceId": "device-demo-001",
  "ts": 1737882000000,
  "values": {
    "charge_power_kw": 1,
    "discharge_power_kw": 2,
    "earnings": 0.1,
    "carbon_reduction": 0.01
  }
}'
```

Verify telemetry:
```bash
psql "$PG_DSN" -c "SELECT tenant_id, station_id, device_id, point_key, ts, value_numeric FROM telemetry_points ORDER BY ts DESC LIMIT 5;"
```

## 3) Close windows → analytics_statistics (HOUR/DAY)

Close hourly windows for 3 days (24h each):
```bash
base_day="2026-01-20"
for day in 0 1 2; do
  for hour in $(seq 0 23); do
    ts=$(date -u -d "$base_day +$day day +$hour hour +5 min" +"%s000")
    window_start=$(date -u -d "$base_day +$day day +$hour hour" +"%Y-%m-%dT%H:00:00Z")
    ingest_post "{
      \"tenantId\": \"tenant-demo\",
      \"stationId\": \"station-demo-001\",
      \"deviceId\": \"device-demo-001\",
      \"ts\": $ts,
      \"values\": {
        \"charge_power_kw\": 1,
        \"discharge_power_kw\": 2,
        \"earnings\": 0.1,
        \"carbon_reduction\": 0.01
      }
    }" >/dev/null
    curl -sS -X POST http://localhost:8080/analytics/window-close \
      -H "Content-Type: application/json" \
      -H "$AUTH_HEADER" \
      -d "{
        \"stationId\": \"station-demo-001\",
        \"windowStart\": \"$window_start\"
      }" >/dev/null
  done
done
```

Verify analytics (SQL + API):
```bash
psql "$PG_DSN" -c "
SELECT time_type, period_start, charge_kwh, discharge_kwh, is_completed
FROM analytics_statistics
WHERE subject_id='station-demo-001' AND time_type IN ('HOUR','DAY')
ORDER BY period_start ASC;"

curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/stats?station_id=station-demo-001&from=2026-01-20T00:00:00Z&to=2026-01-23T00:00:00Z&granularity=day"
```

## 4) Verify settlements_day auto-generated

```bash
psql "$PG_DSN" -c "
SELECT day_start, energy_kwh, amount, currency, status, version
FROM settlements_day
WHERE tenant_id='tenant-demo' AND station_id='station-demo-001'
ORDER BY day_start ASC;"

curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/settlements?station_id=station-demo-001&from=2026-01-20T00:00:00Z&to=2026-01-23T00:00:00Z"
```

## 5) Statement: generate → freeze → export (pdf/xlsx)

Generate draft:
```bash
curl -sS -X POST http://localhost:8080/api/v1/statements/generate \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{ "station_id": "station-demo-001", "month": "2026-01", "category": "owner", "regenerate": false }'
```

Freeze:
```bash
curl -sS -X POST http://localhost:8080/api/v1/statements/{id}/freeze \
  -H "$AUTH_HEADER"
```

Export:
```bash
mkdir -p var/exports
curl -sS -H "$AUTH_HEADER" -o var/exports/statement.pdf "http://localhost:8080/api/v1/statements/{id}/export.pdf"
curl -sS -H "$AUTH_HEADER" -o var/exports/statement.xlsx "http://localhost:8080/api/v1/statements/{id}/export.xlsx"
ls -l var/exports/statement.pdf var/exports/statement.xlsx
```

Verify in DB:
```bash
psql "$PG_DSN" -c "SELECT id, status, version, total_energy_kwh, total_amount FROM settlement_statements ORDER BY created_at DESC LIMIT 3;"
psql "$PG_DSN" -c "SELECT statement_id, day_start, energy_kwh, amount FROM settlement_statement_items ORDER BY day_start ASC LIMIT 5;"
```

## 6) Alarms: rule → trigger → clear + SSE

Create rule (SQL):
```bash
psql "$PG_DSN" -c "
DELETE FROM alarm_rules WHERE id='rule-demo-001';
INSERT INTO alarm_rules (
  id, tenant_id, station_id, name, semantic, operator, threshold,
  hysteresis, duration_seconds, severity, enabled
) VALUES (
  'rule-demo-001',
  'tenant-demo',
  'station-demo-001',
  'Charge Power High',
  'charge_power_kw',
  '>',
  100,
  5,
  0,
  'high',
  TRUE
);"
```

SSE stream (separate terminal):
```bash
curl -N -H "$AUTH_HEADER" http://localhost:8080/api/v1/alarms/stream
```

Trigger alarm:
```bash
ts=$(date -u +"%s000")
ingest_post "{
  \"tenantId\": \"tenant-demo\",
  \"stationId\": \"station-demo-001\",
  \"deviceId\": \"device-demo-001\",
  \"ts\": $ts,
  \"values\": { \"charge_power_kw\": 120 }
}"
```

Clear alarm (recovery below threshold-hysteresis):
```bash
ts=$(date -u +"%s000")
ingest_post "{
  \"tenantId\": \"tenant-demo\",
  \"stationId\": \"station-demo-001\",
  \"deviceId\": \"device-demo-001\",
  \"ts\": $ts,
  \"values\": { \"charge_power_kw\": 90 }
}"
```

Verify alarms:
```bash
curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/alarms?station_id=station-demo-001&status=active&from=2026-01-20T00:00:00Z&to=2026-01-27T00:00:00Z"
psql "$PG_DSN" -c "SELECT id, status, start_at, end_at, last_value FROM alarms ORDER BY updated_at DESC LIMIT 5;"
```

## 7) Commands: issue → sent → acked/timeout

Issue command:
```bash
curl -sS -X POST http://localhost:8080/api/v1/commands \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{
    "tenant_id": "tenant-demo",
    "station_id": "station-demo-001",
    "device_id": "device-demo-001",
    "command_type": "setPower",
    "payload": { "value": 10 },
    "idempotency_key": "setPower-20260101-001"
  }'
```

Query commands:
```bash
now=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
from=$(date -u -d "-1 hour" +"%Y-%m-%dT%H:%M:%SZ")
curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/commands?station_id=station-demo-001&from=$from&to=$now"
```

If TB returns RPC `{"status":"acked"}`, the command will become `acked`.  
If not, it stays `sent` until timeout scan (or you can update manually for pilot):
```bash
psql "$PG_DSN" -c "UPDATE commands SET status='timeout', error='timeout' WHERE status='sent' AND sent_at < NOW() - INTERVAL '1 minute';"
```

## 8) Backfill: facts update; frozen statement unchanged; regenerate new version

Backfill one hour and force recalculation:
```bash
backfill_ts=$(date -u -d "2026-01-21 06:05:00" +"%s000")
backfill_window="2026-01-21T06:00:00Z"
ingest_post "{
  \"tenantId\": \"tenant-demo\",
  \"stationId\": \"station-demo-001\",
  \"deviceId\": \"device-demo-001\",
  \"ts\": $backfill_ts,
  \"values\": { \"charge_power_kw\": 10, \"discharge_power_kw\": 20 }
}" >/dev/null
curl -sS -X POST http://localhost:8080/analytics/window-close \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d "{ \"stationId\": \"station-demo-001\", \"windowStart\": \"$backfill_window\", \"recalculate\": true }" >/dev/null
```

Verify updates:
```bash
psql "$PG_DSN" -c "
SELECT period_start, charge_kwh, discharge_kwh
FROM analytics_statistics
WHERE subject_id='station-demo-001' AND time_type='DAY' AND period_start='2026-01-21T00:00:00Z';"

psql "$PG_DSN" -c "
SELECT day_start, energy_kwh, amount, version
FROM settlements_day
WHERE tenant_id='tenant-demo' AND station_id='station-demo-001' AND day_start='2026-01-21T00:00:00Z';"
```

Frozen statement remains unchanged. Regenerate for corrected version:
```bash
curl -sS -X POST http://localhost:8080/api/v1/statements/generate \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{ "station_id": "station-demo-001", "month": "2026-01", "category": "owner", "regenerate": true }'
```

Check versions:
```bash
curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/statements?station_id=station-demo-001&month=2026-01&category=owner"
```

## One-click script

Run:
```bash
bash scripts/pilot_e2e.sh
```
