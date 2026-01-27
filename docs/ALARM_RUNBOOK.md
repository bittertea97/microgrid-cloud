# Alarm Runbook (Minimal Closed Loop)

## Prereqs

- Apply migrations:
  - `psql "$PG_DSN" -f migrations/009_alarms.sql`
- Ensure stations/devices/point_mappings are provisioned (see `docs/M3_MASTERDATA.md`).

Auth setup (for API calls):
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

## Create a rule (SQL)

> No rule API yet; use SQL to configure.

```sql
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
);
```

## Ingest telemetry (to trigger alarm)

```bash
ingest_post '{
  "tenantId": "tenant-demo",
  "stationId": "station-demo-001",
  "deviceId": "device-demo-001",
  "ts": 1737882000,
  "values": {
    "charge_power_kw": 120
  }
}'
```

## Query alarms

```bash
curl -H "$AUTH_HEADER" "http://localhost:8080/api/v1/alarms?station_id=station-demo-001&status=active&from=2026-01-26T00:00:00Z&to=2026-01-27T00:00:00Z"
```

## Ack an alarm

```bash
curl -X POST http://localhost:8080/api/v1/alarms/{id}/ack \
  -H "$AUTH_HEADER"
```

## Clear an alarm (manual)

```bash
curl -X POST http://localhost:8080/api/v1/alarms/{id}/clear \
  -H "$AUTH_HEADER"
```

## Clear on recovery (auto)

Send a recovery value below the threshold minus hysteresis (e.g. 90 when threshold=100 and hysteresis=5):

```bash
ingest_post '{
  "tenantId": "tenant-demo",
  "stationId": "station-demo-001",
  "deviceId": "device-demo-001",
  "ts": 1737882300,
  "values": {
    "charge_power_kw": 90
  }
}'
```

## SSE alarm stream

```bash
curl -N -H "$AUTH_HEADER" http://localhost:8080/api/v1/alarms/stream
```

SSE payload example:

```json
{
  "type": "active",
  "alarm": {
    "id": "alarm-...",
    "tenant_id": "tenant-demo",
    "station_id": "station-demo-001",
    "originator_type": "device",
    "originator_id": "device-demo-001",
    "rule_id": "rule-demo-001",
    "status": "active",
    "start_at": "2026-01-26T09:00:00Z",
    "last_value": 120,
    "created_at": "2026-01-26T09:00:00Z",
    "updated_at": "2026-01-26T09:00:00Z"
  }
}
```
