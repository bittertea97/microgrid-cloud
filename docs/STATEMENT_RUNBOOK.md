# Settlement Statement Runbook

This runbook covers generating, freezing, voiding, regenerating, and exporting monthly settlement statements.

## 1) Migrations

```bash
psql "$DATABASE_URL" -f migrations/002_settlement.sql
psql "$DATABASE_URL" -f migrations/008_statements.sql
```

Auth setup:
```bash
export AUTH_JWT_SECRET="dev-secret-change-me"
source scripts/lib_auth.sh
AUTH_HEADER="Authorization: Bearer $(jwt_token_hs256 "$AUTH_JWT_SECRET" tenant-demo admin runbook-user 3600)"
```

## 2) Generate a statement (draft)

```bash
curl -sS -X POST http://localhost:8080/api/v1/statements/generate \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{
    "station_id": "station-demo-001",
    "month": "2026-01",
    "category": "owner",
    "regenerate": false
  }'
```

Response:
```json
{ "statement_id": "stmt-...", "status": "draft", "version": 1 }
```

## 3) Freeze a statement

```bash
curl -sS -X POST http://localhost:8080/api/v1/statements/{id}/freeze \
  -H "$AUTH_HEADER"
```

Response includes `snapshot_hash`. Frozen statements are immutable.

## 4) Void + Regenerate

When backfill occurs after a statement is frozen:
- The frozen statement stays unchanged
- Generate a new version with `regenerate=true`
- (Optional) Void the old version

Void:
```bash
curl -sS -X POST http://localhost:8080/api/v1/statements/{id}/void \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{ "reason": "backfill correction" }'
```

Regenerate:
```bash
curl -sS -X POST http://localhost:8080/api/v1/statements/generate \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{
    "station_id": "station-demo-001",
    "month": "2026-01",
    "category": "owner",
    "regenerate": true
  }'
```

## 5) Query statements

```bash
curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/statements?station_id=station-demo-001&month=2026-01&category=owner"
```

Get one statement + items:
```bash
curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/statements/{id}"
```

## 6) Export

PDF:
```bash
curl -sS -H "$AUTH_HEADER" -o statement.pdf "http://localhost:8080/api/v1/statements/{id}/export.pdf"
```

Excel:
```bash
curl -sS -H "$AUTH_HEADER" -o statement.xlsx "http://localhost:8080/api/v1/statements/{id}/export.xlsx"
```

## 7) Reconciliation

Compare statement totals with facts:
```bash
psql "$DATABASE_URL" -c "
SELECT SUM(energy_kwh), SUM(amount), MIN(currency)
FROM settlements_day
WHERE tenant_id = 'tenant-demo'
  AND station_id = 'station-demo-001'
  AND day_start >= '2026-01-01' AND day_start < '2026-02-01';"

psql "$DATABASE_URL" -c "
SELECT total_energy_kwh, total_amount, currency
FROM settlement_statements
WHERE id = '{id}';"
```
