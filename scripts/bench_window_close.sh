#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8081}"
STATION_ID="${STATION_ID:-station-demo-001}"
WINDOW_START="${WINDOW_START:-2026-01-20T00:00:00Z}"
REQUESTS="${REQUESTS:-100}"

AUTH_SECRET="${AUTH_JWT_SECRET:-dev-secret-change-me}"
TENANT_ID="${TENANT_ID:-tenant-demo}"
ROLE="${ROLE:-admin}"
SUBJECT="${SUBJECT:-bench-user}"
TTL="${TTL:-3600}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib_auth.sh"

TOKEN="$(jwt_token_hs256 "$AUTH_SECRET" "$TENANT_ID" "$ROLE" "$SUBJECT" "$TTL")"

TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT

echo "==> window-close benchmark"
echo "base_url=${BASE_URL} station_id=${STATION_ID} window_start=${WINDOW_START} requests=${REQUESTS}"

for _ in $(seq 1 "$REQUESTS"); do
  curl -s -o /dev/null -w '%{time_total}\n' \
    -X POST "${BASE_URL}/analytics/window-close" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${TOKEN}" \
    -d "{\"stationId\":\"${STATION_ID}\",\"windowStart\":\"${WINDOW_START}\"}" >> "$TMP_FILE"
done

sort -n "$TMP_FILE" -o "$TMP_FILE"

awk '
  { a[NR] = $1 }
  END {
    n = NR
    if (n == 0) { exit 1 }
    p50_idx = int((n * 50 + 99) / 100)
    p95_idx = int((n * 95 + 99) / 100)
    sum = 0
    for (i = 1; i <= n; i++) sum += a[i]
    avg = sum / n
    printf("count=%d avg=%.6fs p50=%.6fs p95=%.6fs\n", n, avg, a[p50_idx], a[p95_idx])
  }
' "$TMP_FILE"
