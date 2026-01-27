# Observability

## Overview
The service exposes Prometheus metrics and ships a Grafana dashboard and Prometheus alert rules for core ingest, eventing, commands, shadowrun, and statements workflows.

## Metrics endpoint
- HTTP: `GET /metrics`
- Location: `main.go` registers `promhttp.Handler()` on `/metrics`.
- Local test server: `tools/fake_tb_server/main.go` also exposes `/metrics` for the fake TB server.

## Local monitoring stack (docker-compose)
The default `docker-compose.yml` already runs Prometheus and Grafana.
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (admin/admin)
- Prometheus config: `deploy/prometheus/prometheus.yml`
- Alert rules: `alerts/energy-platform.rules.yml`
- Dashboards: `dashboards/energy-platform.json`

Grafana provisioning is configured in `deploy/grafana/provisioning/dashboards/dashboards.yml` to load dashboards from `/var/lib/grafana/dashboards`.

## Key metrics
### Ingest
- `platform_ingest_requests_total{result}`
- `platform_ingest_latency_seconds{result}`
- `platform_ingest_errors_total{reason}`

### Eventing
- `platform_event_outbox_pending`
- `platform_event_dlq_count`
- `platform_event_consumer_lag_seconds{consumer}`

### Commands
- `platform_command_requests_total`
- `platform_command_results_total{status}` (acked/failed/timeout)

### Analytics
- `platform_analytics_window_total{result}`
- `platform_analytics_window_latency_seconds{result}`

### Settlement
- `platform_settlement_day_total{result}`
- `platform_settlement_day_latency_seconds{result}`

### Alarms
- `platform_alarm_events_total{event}`

### Shadowrun
- `platform_shadowrun_jobs_total{status}`
- `platform_shadowrun_job_duration_seconds`
- `platform_shadowrun_diff_energy_kwh_max`
- `platform_shadowrun_diff_amount_max`
- `platform_shadowrun_diff_max`
- `platform_shadowrun_reports_total`
- `platform_shadowrun_alerts_total`

### Statements
- `platform_statement_generate_total{result}`
- `platform_statement_generate_latency_seconds{result}`
- `platform_statement_freeze_total{result}`
- `platform_statement_freeze_latency_seconds{result}`
- `platform_statement_export_total{format,result}`
- `platform_statement_export_latency_seconds{format,result}`

## Dashboard
The main dashboard is `dashboards/energy-platform.json` and is auto-loaded by Grafana provisioning when using `docker-compose.yml`.

Manual import (if provisioning is disabled):
1. Open Grafana -> Dashboards -> Import.
2. Upload `dashboards/energy-platform.json`.
3. Select the Prometheus data source (uid: `prometheus`).

## Alerts
Alert rules are defined in `alerts/energy-platform.rules.yml` and loaded by Prometheus via `deploy/prometheus/prometheus.yml`.

### Validate alerting locally
1. Start the stack:
   - `docker compose up -d`
2. Verify Prometheus targets:
   - `http://localhost:9090/targets` (microgrid-cloud should be UP).
3. Trigger a basic alert:
   - Stop the app container to trigger `MicrogridCloudDown`:
     - `docker compose stop app`
4. Trigger ingest error rate:
   - Send a bad request: `curl -X GET http://localhost:8080/ingest/thingsboard/telemetry`
5. Trigger DLQ alert (optional):
   - Insert a row into `dead_letter_events` in Postgres and wait 10 minutes.

Use `http://localhost:9090/alerts` to confirm firing alerts.
