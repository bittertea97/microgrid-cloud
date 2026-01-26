# M3 Query API (JSON + CSV)

All time inputs/outputs are **RFC3339 UTC** (e.g. `2026-01-20T00:00:00Z`).

## 1) Statistics Query

`GET /api/v1/stats`

### Query params
- `station_id` (required): station/subject id
- `from` (required): RFC3339 UTC
- `to` (required): RFC3339 UTC, must be after `from`
- `granularity` (required): `hour` or `day`

### Behavior
- `granularity=hour` → `analytics_statistics.time_type = 'HOUR'`
- `granularity=day`  → `analytics_statistics.time_type = 'DAY'`
- Results sorted by `period_start ASC`

### Response fields (from `analytics_statistics`)
- `subject_id`
- `time_type`
- `time_key`
- `period_start`
- `statistic_id`
- `is_completed`
- `completed_at`
- `charge_kwh`
- `discharge_kwh`
- `earnings`
- `carbon_reduction`
- `created_at`
- `updated_at`

### Curl
```bash
curl -sS "http://localhost:8080/api/v1/stats?station_id=station-demo-001&from=2026-01-20T00:00:00Z&to=2026-01-21T00:00:00Z&granularity=hour"
```

## 2) Settlements Query

`GET /api/v1/settlements`

### Query params
- `station_id` (required)
- `from` (required): RFC3339 UTC
- `to` (required): RFC3339 UTC, must be after `from`

### Behavior
- Reads `settlements_day`
- Results sorted by `day_start ASC`
- Tenant is fixed by service configuration (`TENANT_ID`)

### Response fields (from `settlements_day`)
- `tenant_id`
- `station_id`
- `day_start`
- `energy_kwh`
- `amount`
- `currency`
- `status`
- `version`
- `created_at`
- `updated_at`

### Curl
```bash
curl -sS "http://localhost:8080/api/v1/settlements?station_id=station-demo-001&from=2026-01-20T00:00:00Z&to=2026-01-23T00:00:00Z"
```

## 3) CSV Export (Settlements)

`GET /api/v1/exports/settlements.csv`

### Query params
- `station_id` (required)
- `from` (required): RFC3339 UTC
- `to` (required): RFC3339 UTC, must be after `from`

### Behavior
- `Content-Type: text/csv; charset=utf-8`
- Header row included
- Sorted by `day_start ASC`

### CSV columns
1. `tenant_id`
2. `station_id`
3. `day_start`
4. `energy_kwh`
5. `amount`
6. `currency`
7. `status`
8. `version`
9. `created_at`
10. `updated_at`

### Curl
```bash
curl -sS "http://localhost:8080/api/v1/exports/settlements.csv?station_id=station-demo-001&from=2026-01-20T00:00:00Z&to=2026-01-23T00:00:00Z"
```

## Errors
- `400 Bad Request`: missing/invalid params or invalid time range
- `405 Method Not Allowed`: non-GET requests
- `500 Internal Server Error`: query failures
