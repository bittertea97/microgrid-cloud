# Telemetry ingest MVP

This module provides a minimal ThingsBoard webhook ingest and telemetry query implementation for the analytics hourly statistics flow.

## Configuration

- DATABASE_URL or PG_DSN: Postgres DSN (required)
- HTTP_ADDR: HTTP listen address (default :8080)
- TENANT_ID: tenant id used by analytics telemetry query (default tenant-demo)
- STATION_ID: single-station subject id for analytics stats (default station-demo-001)

## Migrations

Apply SQL:

```
psql "$PG_DSN" -f migrations/001_init.sql
```

## Run

```
go mod tidy
$env:PG_DSN = "postgres://user:pass@localhost:5432/microgrid?sslmode=disable"
go run .
```

## Ingest telemetry

```
curl -X POST http://localhost:8080/ingest/thingsboard/telemetry \
  -H "Content-Type: application/json" \
  -d '{
    "tenantId": "tenant-demo",
    "stationId": "station-demo-001",
    "deviceId": "device-001",
    "ts": 1760000000000,
    "values": {
      "charge_power_kw": 5,
      "discharge_power_kw": 0,
      "earnings": 0.2,
      "carbon_reduction": 0.1
    }
  }'
```

## Close window

```
curl -X POST http://localhost:8080/analytics/window-close \
  -H "Content-Type: application/json" \
  -d '{
    "stationId": "station-demo-001",
    "windowStart": "2026-01-21T10:00:00Z"
  }'
```

## Verify

```
psql "$PG_DSN" -c "SELECT subject_id, time_type, time_key, charge_kwh, discharge_kwh, earnings, carbon_reduction FROM analytics_statistics ORDER BY period_start DESC LIMIT 5;"
```
