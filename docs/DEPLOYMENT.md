# Deployment

This document describes dev, test, and prod deployment flows for microgrid-cloud.

## Common configuration

Required environment variables:

- `DATABASE_URL`: Postgres DSN, e.g. `postgres://user:pass@host:5432/db?sslmode=disable`
- `TB_BASE_URL`: ThingsBoard base URL (compose uses the built-in fake TB server on `http://thingsboard:18080`)
- `AUTH_JWT_SECRET`: JWT signing secret (HS256)
- `INGEST_HMAC_SECRET`: HMAC secret used to sign ingest payloads
- `TB_TOKEN`: ThingsBoard API token (optional if your endpoints do not call TB APIs)

Optional environment variables:

- `HTTP_ADDR` (default `:8080`)
- `TENANT_ID` (default `tenant-demo`)
- `STATION_ID` (default `station-demo-001`)
- `PRICE_PER_KWH` (default `1.0`)
- `CURRENCY` (default `CNY`)
- `EXPECTED_HOURS` (default `24`)
- `INGEST_MAX_SKEW_SECONDS` (default `300`)

Database migrations are applied with the `migrate/migrate` CLI using the SQL files in `migrations/`.
In dev/test, migrations run automatically via the `migrate` init container in compose.

## Dev (docker compose)

Uses local containers for Postgres, NATS, and MinIO. Prometheus/Grafana are optional via a profile.

Steps:

1) Start the stack

```
docker compose -f docker-compose.dev.yml up -d
```

2) Optional: start monitoring stack

```
docker compose -f docker-compose.dev.yml --profile monitoring up -d
```

3) Verify services

- API: `http://localhost:8080/healthz`
- Grafana: `http://localhost:3000` (admin/admin) (monitoring profile only)
- Prometheus: `http://localhost:9090` (monitoring profile only)
- MinIO: `http://localhost:9001` (minio/minio123)

Notes:

- `migrate` runs once at startup and the app waits for it to complete.
- The compose files start a fake ThingsBoard server for provisioning/RPC. For real integrations, override `TB_BASE_URL` + `TB_TOKEN`.
- Prometheus is configured to scrape `/metrics` from the app. If you do not expose metrics yet, the target will show as down.

### One-click local validation (pilot/shadowrun)

Prereqs on your workstation: `bash`, `curl`, `jq` or `python`, `psql`, and GNU `date` (Git Bash is fine on Windows).

```
BASE_URL=http://localhost:8080 \
PG_DSN=postgres://microgrid:microgrid@localhost:5432/microgrid?sslmode=disable \
AUTH_JWT_SECRET=dev-secret-change-me \
INGEST_HMAC_SECRET=dev-ingest-secret \
bash scripts/pilot_e2e.sh

BASE_URL=http://localhost:8080 \
PG_DSN=postgres://microgrid:microgrid@localhost:5432/microgrid?sslmode=disable \
AUTH_JWT_SECRET=dev-secret-change-me \
bash scripts/shadowrun_local.sh
```

Windows / PowerShell release-check:

```powershell
.\scripts\release_acceptance.ps1 `
  -BaseUrl "http://localhost:8080" `
  -PgDsn "postgres://microgrid:microgrid@localhost:5432/microgrid?sslmode=disable" `
  -AuthJwtSecret "dev-secret-change-me" `
  -IngestHmacSecret "dev-ingest-secret"
```

## Test (docker compose)

Test uses a separate database and does not publish ports by default.

Steps:

1) Start the test stack

```
docker compose -f docker-compose.dev.yml -f docker-compose.test.yml up -d
```

2) Run tests from your workstation or CI as usual

```
go test ./...
```

## Prod (container runtime / orchestrator)

In production, use managed Postgres/NATS/MinIO where possible. Prometheus and Grafana are optional.

1) Build/pull the image

- CI publishes to the configured registry.
- Example image: `ghcr.io/<org>/microgrid-cloud:<tag>`
- Tag rules: `sha-<short>` is always produced; `X.Y.Z` is produced when pushing a `vX.Y.Z` tag.

2) Run database migrations (one-off job or init container)

```
docker run --rm \
  -v $(pwd)/migrations:/migrations \
  migrate/migrate:v4.16.2 \
  -path /migrations \
  -database "postgres://user:pass@host:5432/db?sslmode=disable" \
  up
```

3) Start the app

```
docker run -d \
  -e DATABASE_URL="postgres://user:pass@host:5432/db?sslmode=disable" \
  -e TB_BASE_URL="https://tb.example.com" \
  -e TB_TOKEN="<token>" \
  -e AUTH_JWT_SECRET="<jwt-secret>" \
  -e INGEST_HMAC_SECRET="<ingest-secret>" \
  -p 8080:8080 \
  ghcr.io/<org>/microgrid-cloud:<tag>
```

## Dev/Test/Prod differences

- **Dev**: local services, ports published, uses `docker-compose.dev.yml`.
- **Test**: separate DB (`microgrid_test`), no host ports (override in `docker-compose.test.yml`).
- **Prod**: external services, explicit env vars, migrations run as a one-off job or init container before app start.

## Release artifact contents (CI)

The CI pipeline publishes a `microgrid-release` artifact containing:

- `migrations/`
- `dashboards/`
- `runbooks/`
