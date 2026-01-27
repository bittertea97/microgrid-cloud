# Strategy Runbook (Anti-Backflow MVP)

This runbook configures and validates the anti-backflow strategy with calendar window and auto/manual mutual exclusion.

All time inputs/outputs use RFC3339 UTC.

## 1) Migrations

```bash
psql "$PG_DSN" -f migrations/001_init.sql
psql "$PG_DSN" -f migrations/003_masterdata.sql
psql "$PG_DSN" -f migrations/005_eventing.sql
psql "$PG_DSN" -f migrations/007_commands.sql
psql "$PG_DSN" -f migrations/013_strategy.sql
```

## 2) Seed point mapping (grid_export_kw)

The anti-backflow strategy reads the semantic `grid_export_kw`.

```bash
psql "$PG_DSN" -c "
INSERT INTO point_mappings (
  id, station_id, device_id, point_key, semantic, unit, factor
) VALUES (
  'map-grid-export-001',
  'station-demo-001',
  'device-demo-001',
  'grid_export_kw',
  'grid_export_kw',
  'kW',
  1
)
ON CONFLICT (id) DO UPDATE SET updated_at = NOW();"
```

Auth setup:
```bash
export AUTH_JWT_SECRET="dev-secret-change-me"
export INGEST_HMAC_SECRET="dev-ingest-secret"
source scripts/lib_auth.sh
AUTH_HEADER="Authorization: Bearer $(jwt_token_hs256 "$AUTH_JWT_SECRET" tenant-demo operator runbook-user 3600)"
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

## 3) Configure strategy: mode + enable + calendar

Set mode to auto:
```bash
curl -sS -X POST http://localhost:8080/api/v1/strategies/station-demo-001/mode \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{ "mode": "auto" }'
```

Enable strategy (template params):
```bash
curl -sS -X POST http://localhost:8080/api/v1/strategies/station-demo-001/enable \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{
    "enabled": true,
    "template_type": "anti_backflow",
    "template_params": {
      "threshold_kw": 10,
      "min_kw": 0,
      "max_kw": 100,
      "device_id": "device-demo-001",
      "command_type": "setPower"
    }
  }'
```

Set today’s calendar window:
```bash
today=$(date -u +"%Y-%m-%d")
curl -sS -X POST http://localhost:8080/api/v1/strategies/station-demo-001/calendar \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d "{
    \"date\": \"$today\",
    \"enabled\": true,
    \"start_time\": \"00:00\",
    \"end_time\": \"23:59\"
  }"
```

Manual/auto mutual exclusion (MVP):
- If mode is `manual`, auto strategy will not issue commands.
- Manual commands are still accepted; only auto is gated.

## 4) Trigger telemetry and validate command issuance

Send grid export value above threshold:
```bash
ts=$(date -u +"%s000")
ingest_post "{
  \"tenantId\": \"tenant-demo\",
  \"stationId\": \"station-demo-001\",
  \"deviceId\": \"device-demo-001\",
  \"ts\": $ts,
  \"values\": { \"grid_export_kw\": 50 }
}"
```

The strategy tick runs every minute. Wait up to 60s, then verify commands:
```bash
from=$(date -u -d "-5 minutes" +"%Y-%m-%dT%H:%M:%SZ")
to=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
curl -sS "http://localhost:8080/api/v1/commands?station_id=station-demo-001&from=$from&to=$to" \
  -H "$AUTH_HEADER"
```

Verify strategy runs:
```bash
curl -sS "http://localhost:8080/api/v1/strategies/station-demo-001/runs?from=$from&to=$to" \
  -H "$AUTH_HEADER"
```

## 5) Switch to manual mode (auto disabled)

```bash
curl -sS -X POST http://localhost:8080/api/v1/strategies/station-demo-001/mode \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{ "mode": "manual" }'
```

Now auto strategy will not issue commands.
