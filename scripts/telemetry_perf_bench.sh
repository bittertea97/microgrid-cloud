#!/usr/bin/env bash
set -euo pipefail

DSN="${PG_DSN:-${DATABASE_URL:-}}"
if [[ -z "${DSN}" ]]; then
  echo "PG_DSN or DATABASE_URL is required" >&2
  exit 1
fi

echo "==> telemetry perf: insert 30d + query 7d"
PG_DSN="${DSN}" go test -run TestTelemetryPerf_30dInsert_7dQuery -v ./internal/telemetry/integration
