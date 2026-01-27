# M2 Runbook (Day Rollup + Day Settlement)

This runbook validates the M2 closed loop locally with 1 station + 3 days of data, and verifies backfill consistency.

## 1) Start Postgres

```bash
docker run --name microgrid-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 -d postgres:15
```

Create DB and apply migrations:

```bash
psql "postgres://postgres:postgres@localhost:5432/postgres" -c "CREATE DATABASE microgrid;"

export PG_DSN="postgres://postgres:postgres@localhost:5432/microgrid?sslmode=disable"
psql "$PG_DSN" -f migrations/001_init.sql
psql "$PG_DSN" -f migrations/002_settlement.sql
```

## 2) Run service

```bash
export DATABASE_URL="$PG_DSN"
export TENANT_ID="tenant-demo"
export STATION_ID="station-demo-001"
export PRICE_PER_KWH="1.0"
export EXPECTED_HOURS="24"
export AUTH_JWT_SECRET="dev-secret-change-me"
export INGEST_HMAC_SECRET="dev-ingest-secret"
go run .
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

## 3) Ingest telemetry (3 days) + close windows

The service expects ThingsBoard-style telemetry payloads, and a window-close event per hour.

```bash
tenant="tenant-demo"
station="station-demo-001"
device="device-001"

base_day="2026-01-20"
for day in 0 1 2; do
  for hour in $(seq 0 23); do
    ts=$(date -u -d "$base_day +$day day +$hour hour +5 min" +"%s000")
    window_start=$(date -u -d "$base_day +$day day +$hour hour" +"%Y-%m-%dT%H:00:00Z")

    ingest_post "{
      \"tenantId\": \"$tenant\",
      \"stationId\": \"$station\",
      \"deviceId\": \"$device\",
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
        \"stationId\": \"$station\",
        \"windowStart\": \"$window_start\"
      }" >/dev/null
  done
done
```

## 4) Verify day statistics + settlements

```bash
psql "$PG_DSN" -c "
SELECT time_type, period_start, charge_kwh, discharge_kwh, is_completed
FROM analytics_statistics
WHERE subject_id = 'station-demo-001' AND time_type = 'DAY'
ORDER BY period_start ASC;"

psql "$PG_DSN" -c "
SELECT day_start, energy_kwh, amount, currency, status, version
FROM settlements_day
WHERE tenant_id = 'tenant-demo' AND station_id = 'station-demo-001'
ORDER BY day_start ASC;"
```

You should see 3 DAY rows in `analytics_statistics` and 3 rows in `settlements_day`.

## 5) Backfill one hour and re-run window close

```bash
backfill_ts=$(date -u -d "2026-01-21 06:05:00" +"%s000")
backfill_window="2026-01-21T06:00:00Z"

ingest_post "{
  \"tenantId\": \"tenant-demo\",
  \"stationId\": \"station-demo-001\",
  \"deviceId\": \"device-001\",
  \"ts\": $backfill_ts,
  \"values\": {
    \"charge_power_kw\": 10,
    \"discharge_power_kw\": 20,
    \"earnings\": 0.1,
    \"carbon_reduction\": 0.01
  }
}" >/dev/null

curl -sS -X POST http://localhost:8080/analytics/window-close \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d "{
    \"stationId\": \"station-demo-001\",
    \"windowStart\": \"$backfill_window\",
    \"recalculate\": true
  }" >/dev/null
```

Re-check:

```bash
psql "$PG_DSN" -c "
SELECT period_start, charge_kwh, discharge_kwh
FROM analytics_statistics
WHERE subject_id = 'station-demo-001' AND time_type = 'DAY' AND period_start = '2026-01-21T00:00:00Z';"

psql "$PG_DSN" -c "
SELECT day_start, energy_kwh, amount, version
FROM settlements_day
WHERE tenant_id = 'tenant-demo' AND station_id = 'station-demo-001' AND day_start = '2026-01-21T00:00:00Z';"
```

The DAY statistic and the settlement row should both reflect the backfilled energy, and `version` should increment.
