# Provisioning + TB Adapter Runbook

This runbook describes how to provision a station and its TB mappings via the platform API.

## 1) Prerequisites

Environment:
```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/microgrid?sslmode=disable"
export TB_BASE_URL="http://localhost:18080"     # TB REST base URL (fake TB server in compose)
export TB_TOKEN="your-tb-token"
export AUTH_JWT_SECRET="dev-secret-change-me"
source scripts/lib_auth.sh
AUTH_HEADER="Authorization: Bearer $(jwt_token_hs256 "$AUTH_JWT_SECRET" tenant-demo admin runbook-user 3600)"
```

Run migrations:
```bash
psql "$DATABASE_URL" -f migrations/001_init.sql
psql "$DATABASE_URL" -f migrations/003_masterdata.sql
psql "$DATABASE_URL" -f migrations/006_provisioning.sql
```

## 2) Provision a station

```bash
curl -sS -X POST http://localhost:8080/api/v1/provisioning/stations \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{
    "station": {
      "tenant_id": "tenant-demo",
      "name": "station-demo-001",
      "timezone": "UTC",
      "type": "microgrid",
      "region": "lab"
    },
    "devices": [
      {
        "name": "device-inverter-001",
        "device_type": "inverter",
        "tb_profile": "default",
        "credentials": "token-123"
      }
    ],
    "point_mappings": [
      {
        "point_key": "charge_power_kw",
        "semantic": "charge_power_kw",
        "unit": "kW",
        "factor": 1
      },
      {
        "point_key": "discharge_power_kw",
        "semantic": "discharge_power_kw",
        "unit": "kW",
        "factor": 1
      }
    ]
  }'
```

Response example:
```json
{
  "station_id": "station-xxxx",
  "tb": {
    "tenant_id": "tb-tenant-id",
    "asset_id": "tb-asset-id",
    "devices": [
      {
        "device_id": "device-xxxx",
        "tb_device_id": "tb-device-id",
        "credentials": "token-123"
      }
    ]
  }
}
```

Repeated calls with the same payload are idempotent.

## 3) Validate in DB

```bash
psql "$DATABASE_URL" -c "SELECT id, tenant_id, tb_asset_id, tb_tenant_id FROM stations;"
psql "$DATABASE_URL" -c "SELECT id, station_id, tb_entity_id FROM devices;"
psql "$DATABASE_URL" -c "SELECT station_id, point_key, semantic, unit, factor FROM point_mappings;"
```

## 4) Validate in TB

In ThingsBoard:
- Tenant exists (name = tenant_id)
- Asset exists for the station
- Device exists and is related to the asset
- Asset attributes include `station_id`, `tenant_id`
- Device attributes include `device_id`, `station_id`
