# Command Runbook (TB RPC)

This document explains how to issue commands via the platform and how TB RPC is used for delivery.

## Migrations

```bash
psql "$DATABASE_URL" -f migrations/005_eventing.sql
psql "$DATABASE_URL" -f migrations/007_commands.sql
```

## API

Auth setup:
```bash
export AUTH_JWT_SECRET="dev-secret-change-me"
source scripts/lib_auth.sh
AUTH_HEADER="Authorization: Bearer $(jwt_token_hs256 "$AUTH_JWT_SECRET" tenant-demo operator runbook-user 3600)"
```

### Issue command

`POST /api/v1/commands`

Payload:
```json
{
  "tenant_id": "tenant-demo",
  "station_id": "station-demo-001",
  "device_id": "device-inverter-001",
  "command_type": "setPower",
  "payload": { "value": 10 },
  "idempotency_key": "setPower-20260101-001"
}
```

If `idempotency_key` is omitted, the platform will generate one based on payload.

Example:
```bash
curl -sS -X POST http://localhost:8080/api/v1/commands \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{ "tenant_id": "tenant-demo", "station_id": "station-demo-001", "device_id": "device-inverter-001", "command_type": "setPower", "payload": { "value": 10 }, "idempotency_key": "setPower-20260101-001" }'
```

### Query commands

`GET /api/v1/commands?station_id=...&from=...&to=...`

Time format: RFC3339 UTC

Example:
```bash
curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/commands?station_id=station-demo-001&from=2026-01-26T00:00:00Z&to=2026-01-27T00:00:00Z"
```

## TB RPC Mapping

The TB adapter sends:
```
POST /api/rpc/{deviceId}
{
  "method": "<command_type>",
  "params": <payload>
}
```

Response:
- `{"status":"acked"}` => command is marked `acked`
- `{"status":"sent"}`  => command stays `sent` until timeout scan
- `{"status":"failed","error":"..."}` => command is marked `failed`

## Timeout Scan

Commands in `sent` status can be marked timeout:
- Update logic: `sent_at < now - timeout`
- Status becomes `timeout`

## Notes

- Idempotency: same `idempotency_key` within 10 minutes returns existing command (no duplicate RPC).
- Command events: `CommandIssued`, `CommandAcked`, `CommandFailed` are emitted to outbox.
