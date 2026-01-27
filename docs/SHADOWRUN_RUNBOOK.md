# Shadow Run Pipeline Runbook

This runbook describes the daily shadow run reconciliation pipeline, report generation, and alerting.

All time inputs/outputs use RFC3339 UTC.

## 1) Migrations

```bash
for f in $(ls migrations/*.sql | sort); do
  psql "$PG_DSN" -f "$f"
done
```

## 2) Configuration

Environment variables:
```bash
export SHADOWRUN_STORAGE_ROOT="var/reports/shadowrun"
export SHADOWRUN_PUBLIC_BASE_URL="http://localhost:8080"
export SHADOWRUN_DAILY_AT="02:00"
export SHADOWRUN_STATIONS="station-demo-001,station-demo-002"
export SHADOWRUN_WEBHOOK_URL="https://webhook.example.com/..."
```

Auth setup (required for API calls):
```bash
export AUTH_JWT_SECRET="dev-secret-change-me"
source scripts/lib_auth.sh
AUTH_HEADER="Authorization: Bearer $(jwt_token_hs256 "$AUTH_JWT_SECRET" tenant-demo admin runbook-user 3600)"
```

Optional YAML config:
```yaml
defaults:
  energy_abs: 5
  energy_pct: 0.05
  amount_abs: 5
  amount_pct: 0.05
  missing_hours: 2
schedule:
  daily_at: "02:00"
  stations:
    - station-demo-001
stations:
  station-demo-001:
    energy_abs: 2
storage_root: "var/reports/shadowrun"
public_base_url: "http://localhost:8080"
webhook_url: "https://webhook.example.com/..."
fallback_price: 1.0
```

Enable YAML via:
```bash
export SHADOWRUN_CONFIG="./config/shadowrun.yaml"
```

## 3) Scheduler policy

Default policy:
- Daily at `02:00` UTC
- Runs **month-to-date** for each station in `SHADOWRUN_STATIONS`
- Job date = current UTC date (used for idempotency)

Idempotency:
- Same `tenant_id + station_id + month + job_date` will not create duplicates.

## 4) Manual trigger API

```bash
curl -sS -X POST http://localhost:8080/api/v1/shadowrun/run \
  -H "Content-Type: application/json" \
  -H "$AUTH_HEADER" \
  -d '{
    "tenant_id": "tenant-demo",
    "station_ids": ["station-demo-001"],
    "month": "2026-01",
    "thresholds": {
      "energy_abs": 5,
      "amount_abs": 5,
      "missing_hours": 2
    }
  }'
```

## 5) Reports query & download

List reports:
```bash
curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/shadowrun/reports?station_id=station-demo-001&from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z"
```

Get report metadata:
```bash
curl -sS -H "$AUTH_HEADER" "http://localhost:8080/api/v1/shadowrun/reports/{id}"
```

Download report (zip contains CSV/JSON):
```bash
curl -sS -H "$AUTH_HEADER" -o shadowrun_report.zip "http://localhost:8080/api/v1/shadowrun/reports/{id}/download"
```

## 6) Alerting

When any diff exceeds thresholds:
- A row is inserted into `shadowrun_alerts` (compat view: `system_alerts`)
- A webhook notification is sent (text payload)

Suggested actions included:
- `replay_missing_hours`
- `check_mapping_or_tariff`
- `check_tariff_or_settlement`

## 7) Replay/Backfill

API:
```bash
curl -sS -X POST "http://localhost:8080/api/v1/shadowrun/reports/{id}/replay" \
  -H "$AUTH_HEADER"
```

Current status: recorded as a TODO job (replay pipeline not implemented yet).

## 8) Metrics

Prometheus endpoint:
```
GET /metrics
```

Metrics:
- `platform_shadowrun_jobs_total{status}`
- `platform_shadowrun_job_duration_seconds`
- `platform_shadowrun_diff_energy_kwh_max`
- `platform_shadowrun_diff_amount_max`
- `platform_shadowrun_reports_total`
- `platform_shadowrun_alerts_total`

## 9) Local one-click script

```bash
bash scripts/shadowrun_local.sh
```
