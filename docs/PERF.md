# Performance & Capacity (Ingest / Stats / Statement / Command)

This document describes how to run the performance tests and how to interpret the results.

## Scope
- Ingest write QPS (simulate station + point counts)
- Stats query concurrency
- Statement export concurrency (PDF/XLSX)
- Command issuance concurrency (uses fake ThingsBoard RPC server)

## Prerequisites
1) Migrations applied (at least):
   - `migrations/001_init.sql`
   - `migrations/005_eventing.sql`
   - `migrations/007_commands.sql`
   - `migrations/008_statements.sql`
2) Service running with TB base URL configured (required by `main.go`):
   - `TB_BASE_URL` must be set, use fake server below for perf.
3) Install `k6` (one of):
   - Windows: `choco install k6`
   - macOS: `brew install k6`
   - Linux: package manager or https://k6.io/docs/get-started/installation/

## Fake ThingsBoard RPC server (for command tests)
Run in a separate terminal:

```powershell
$env:FAKE_TB_ADDR=":18080"
$env:FAKE_TB_STATUS="acked"  # acked|sent|failed (optional)
$env:FAKE_TB_LATENCY_MS="0"  # simulate TB latency
$env:FAKE_TB_FAIL_RATE="0"    # 0.0~1.0 optional
$env:FAKE_TB_SENT_RATE="0"    # 0.0~1.0 optional

go run .\tools\fake_tb_server
```

Then start the platform:

```powershell
$env:PG_DSN="postgres://user:pass@localhost:5432/microgrid?sslmode=disable"
$env:TB_BASE_URL="http://localhost:18080"
$env:TB_TOKEN="dummy"

go run .
```

## Seed data (for stats / statements)
Stats and statements require data. Use one of:
- `scripts/pilot_e2e.sh` (full flow)
- `docs/M2_RUNBOOK.md` or `docs/PILOT_RUNBOOK.md` to ingest telemetry and generate stats/settlements

For statement export tests, ensure you have statement IDs (see `docs/STATEMENT_RUNBOOK.md`).

### Bulk seed tool (recommended for perf)
This tool seeds `analytics_statistics` + `settlements_day`, and can optionally generate statements + output IDs.

```powershell
$env:PG_DSN="postgres://user:pass@localhost:5432/microgrid?sslmode=disable"
$env:BASE_URL="http://localhost:8080"

go run .\tools\perf_seed `
  -pg-dsn $env:PG_DSN `
  -tenant-id "tenant-demo" `
  -station-prefix "station-perf-" `
  -station-count 200 `
  -start-date "2026-01-01" `
  -days 30 `
  -seed-hourly=true `
  -seed-daily=true `
  -seed-settlements=true `
  -generate-statements=true `
  -statement-month "2026-01" `
  -statement-category "owner" `
  -statement-ids-out "reports/perf/statement_ids.txt"
```

Use `reports/perf/statement_ids.txt` with the statement export test (see below).

## Load tests (k6)
### 1) Ingest QPS
Simulates stations + devices + points per device, with a target QPS.

```powershell
$outDir = "reports/perf/$(Get-Date -Format 'yyyyMMdd-HHmmss')/ingest"
New-Item -ItemType Directory -Force $outDir | Out-Null
k6 run --summary-export "$outDir/summary.json" --out json="$outDir/metrics.json" `
  -e BASE_URL="http://localhost:8080" `
  -e TENANT_ID="tenant-demo" `
  -e STATION_COUNT=100 `
  -e DEVICES_PER_STATION=5 `
  -e POINTS_PER_DEVICE=20 `
  -e INGEST_RPS=1000 `
  -e DURATION="5m" `
  scripts/perf/k6/ingest.js
```

Key envs: `STATION_COUNT`, `DEVICES_PER_STATION`, `POINTS_PER_DEVICE`, `INGEST_RPS`, `DURATION`.

### 2) Stats query concurrency
```powershell
$outDir = "reports/perf/$(Get-Date -Format 'yyyyMMdd-HHmmss')/stats"
New-Item -ItemType Directory -Force $outDir | Out-Null
k6 run --summary-export "$outDir/summary.json" --out json="$outDir/metrics.json" `
  -e BASE_URL="http://localhost:8080" `
  -e VUS=50 `
  -e DURATION="5m" `
  -e STATION_COUNT=100 `
  -e FROM_TS="2026-01-20T00:00:00Z" `
  -e TO_TS="2026-01-21T00:00:00Z" `
  -e GRANULARITY="hour" `
  scripts/perf/k6/stats.js
```

Key envs: `VUS`, `DURATION`, `FROM_TS`, `TO_TS`, `GRANULARITY`.

### 3) Statement export concurrency
Provide `STATEMENT_ID` or `STATEMENT_IDS` (comma-separated):

```powershell
$outDir = "reports/perf/$(Get-Date -Format 'yyyyMMdd-HHmmss')/statement_export"
New-Item -ItemType Directory -Force $outDir | Out-Null
k6 run --summary-export "$outDir/summary.json" --out json="$outDir/metrics.json" `
  -e BASE_URL="http://localhost:8080" `
  -e VUS=20 `
  -e DURATION="5m" `
  -e STATEMENT_IDS="stmt-001,stmt-002" `
  -e EXPORT_FORMATS="pdf,xlsx" `
  scripts/perf/k6/statement_export.js
```

Or use a file:

```powershell
k6 run -e BASE_URL="http://localhost:8080" `
  -e VUS=20 `
  -e DURATION="5m" `
  -e STATEMENT_IDS_FILE="reports/perf/statement_ids.txt" `
  -e EXPORT_FORMATS="pdf,xlsx" `
  scripts/perf/k6/statement_export.js
```

### 4) Command issuance concurrency
Requires fake TB server running + `TB_BASE_URL` in the service.

```powershell
$outDir = "reports/perf/$(Get-Date -Format 'yyyyMMdd-HHmmss')/commands"
New-Item -ItemType Directory -Force $outDir | Out-Null
k6 run --summary-export "$outDir/summary.json" --out json="$outDir/metrics.json" `
  -e BASE_URL="http://localhost:8080" `
  -e TENANT_ID="tenant-demo" `
  -e STATION_COUNT=100 `
  -e DEVICES_PER_STATION=5 `
  -e VUS=50 `
  -e DURATION="5m" `
  -e COMMAND_TYPE="setPower" `
  scripts/perf/k6/commands.js
```

## Metrics collection
### App CPU / Memory
- Windows: `Get-Process -Name microgrid-cloud | Select-Object CPU,WorkingSet,PM,StartTime`
- Linux: `top`, `htop`, or `ps -o pid,pcpu,pmem,rsz,cmd -p <pid>`
- Docker: `docker stats <container>`

### DB write TPS + backlog
Use `scripts/perf/pg_metrics.sql` before and after the test:

```powershell
$outDir = "reports/perf/$(Get-Date -Format 'yyyyMMdd-HHmmss')/db"
New-Item -ItemType Directory -Force $outDir | Out-Null
psql "$env:PG_DSN" -f scripts/perf/pg_metrics.sql > "$outDir/pg_metrics_before.txt"
# run load test...
psql "$env:PG_DSN" -f scripts/perf/pg_metrics.sql > "$outDir/pg_metrics_after.txt"
```

Compute:
- **DB TPS**: `(xact_commit + xact_rollback) delta / test_seconds`
- **Write TPS**: `(tup_inserted + tup_updated + tup_deleted) delta / test_seconds`
- **Queue backlog**: `event_outbox` rows by status + `dead_letter_events` count

### Queue backlog interpretation
- `event_outbox.status='pending'` increasing during test indicates dispatch lag
- `dead_letter_events` > 0 indicates failures (investigate)

### App /metrics snapshot (optional)
```powershell
$outDir = "reports/perf/$(Get-Date -Format 'yyyyMMdd-HHmmss')/app"
New-Item -ItemType Directory -Force $outDir | Out-Null
Invoke-WebRequest -Uri "http://localhost:8080/metrics" -OutFile "$outDir/metrics.prom"
```

## Report
Fill in `docs/PERF_REPORT_TEMPLATE.md` after each run and link raw artifacts:
- k6 `summary.json` + `metrics.json`
- DB snapshots (`pg_metrics_before.txt` / `pg_metrics_after.txt`)
- `/metrics` snapshot (if collected)

## Capacity guidance (fill after tests)
Use sustained (non-error) throughput and P95 to calculate capacity.

- **Per-instance ingest points/sec**: `ingest_rps * points_per_device`
- **Per-station ingest rps**: `devices_per_station * sample_rate_hz`
- **Max stations per instance**: `sustained_ingest_rps / per_station_rps`
- **Max points per instance**: `sustained_ingest_rps * points_per_device`

DB tuning checklist (adjust based on DB TPS + latency):
- Connection pool (app): `MaxOpenConns` ~= 2-4x vCPU, `MaxIdleConns` ~= 50-80% of MaxOpen, `ConnMaxLifetime` 5-15m
- Connection limits (DB): `max_connections` >= app pool + admin + background with 20% headroom
- Indexes: ensure `idx_telemetry_points_station_ts`, `idx_analytics_statistics_period`, `idx_settlements_day_station`, `idx_commands_station_time` exist (migrations already create them)
- If stats/export is slow, consider covering indexes on `(subject_id, time_type, period_start)` and `(tenant_id, station_id, day_start)`
- WAL / checkpoints: raise `max_wal_size` or `checkpoint_timeout` if write TPS is high and checkpoints dominate latency
