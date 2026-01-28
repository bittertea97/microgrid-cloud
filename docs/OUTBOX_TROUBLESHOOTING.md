# Outbox Troubleshooting (Dev)

## Symptoms
- `/analytics/window-close` requests are slow (seconds to minutes).
- App logs show long durations for `POST /analytics/window-close`.

## Why it happens
The request handler publishes `TelemetryWindowClosed` into the outbox and then
**synchronously dispatches one pending outbox event**. If the outbox has a large
pending backlog, each request waits for dispatch + downstream handlers. This
combines with DB write latency (WAL sync) to make the endpoint slow.

## Cleanup (Dev Only)
Use `scripts/outbox_cleanup.sql` to inspect and clear pending rows.

### 1) Inspect pending counts
```bash
docker exec -i microgrid-cloud-dev-postgres-1 \
  psql -U microgrid -d microgrid \
  -v action=stats \
  < scripts/outbox_cleanup.sql
```

### 2) Delete pending rows for a specific event_type
```bash
docker exec -i microgrid-cloud-dev-postgres-1 \
  psql -U microgrid -d microgrid \
  -v action=cleanup_type \
  -v event_type='events.StatisticCalculated' \
  -v confirm=YES \
  < scripts/outbox_cleanup.sql
```

### 3) Delete all pending rows (dev only)
```bash
docker exec -i microgrid-cloud-dev-postgres-1 \
  psql -U microgrid -d microgrid \
  -v action=cleanup_all \
  -v confirm=YES \
  < scripts/outbox_cleanup.sql
```

## Verify improvement
Re-run a window close request and compare log latency:
```bash
TOKEN="$(source scripts/lib_auth.sh; jwt_token_hs256 dev-secret-change-me tenant-demo admin runbook-user 3600)"
curl -sS -X POST http://localhost:8081/analytics/window-close \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"stationId":"station-demo-001","windowStart":"2026-01-20T00:00:00Z"}'
```
