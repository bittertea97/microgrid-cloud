# Runbook Index

This folder contains operational runbooks for microgrid-cloud. All API calls require JWT auth; ingest endpoints require HMAC signatures.

## Common prerequisites
- Tools: `bash`, `curl`, `jq` or `python`, `psql`, `openssl`, GNU `date` (or `gdate` on macOS)
- Env: `AUTH_JWT_SECRET`, `INGEST_HMAC_SECRET`, `DATABASE_URL`/`PG_DSN`, `TB_BASE_URL`
- Auth helper: `source scripts/lib_auth.sh`

## Runbooks
- `DEPLOYMENT.md` ? Dev/Test/Prod setup + one-click validation (pilot/shadowrun)
- `SECURITY.md` ? Auth, RBAC, tenant isolation, audit logging
- `OBSERVABILITY.md` ? Metrics, dashboards, alert rules
- `PG_RETENTION.md` ? Telemetry partitioning, retention, maintenance
- `PILOT_RUNBOOK.md` ? End-to-end pilot flow (provision ? ingest ? stats/settlement ? statements ? alarms ? commands)
- `SHADOWRUN_RUNBOOK.md` ? Shadowrun reconciliation + alerts
- `PROVISIONING_RUNBOOK.md` ? Station/device provisioning + TB adapter
- `COMMAND_RUNBOOK.md` ? Command issue/query + TB RPC mapping
- `STATEMENT_RUNBOOK.md` ? Statement generate/freeze/void/export
- `STRATEGY_RUNBOOK.md` ? Strategy configuration + trigger
- `M2_RUNBOOK.md` ? Day rollup + settlement validation
- `M3_QUERY_API.md` ? Stats/settlements query + CSV export
- `M3_MASTERDATA.md` ? Point mappings and semantics
- `M3_TARIFF.md` ? Tariff inputs
- `M4_EVENTING.md` ? Outbox/DLQ/processed events
- `ALARM_RUNBOOK.md` ? Alarm rule + trigger/clear
- `PERF.md` / `PERF_REPORT_TEMPLATE.md` ? Perf guidance and reporting

## Release acceptance
- Script (Linux/macOS): `scripts/release_acceptance.sh`
- Script (Windows/PowerShell): `scripts/release_acceptance.ps1`
- Covers: migrations, provisioning, ingest + analytics, settlements/statements, alarms, commands, shadowrun
- Output: summary report + logs under `var/`
