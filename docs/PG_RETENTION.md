# Postgres Telemetry Partition & Retention

## Strategy

- Table: `telemetry_points` is partitioned by **month** on `ts` using native declarative partitioning.
- Partition naming: `telemetry_points_YYYYMM`.
- Default partition: `telemetry_points_default` catches any unexpected timestamps.
- Retention: drop partitions older than `RETENTION_DAYS` (default 90).

## Migration

### New environments (recommended)

Apply only the monthly partition migration:

```bash
psql "$PG_DSN" -f migrations/015_telemetry_partition.sql
```

**Do not** apply the legacy daily migration (`010_telemetry_partition.sql`) on new environments.

### Existing environments (legacy daily partitions)

If you already applied `010_telemetry_partition.sql`, upgrade during a maintenance window:

1) Ensure recent backups.
2) Apply the monthly migration once:

```bash
psql "$PG_DSN" -f migrations/015_telemetry_partition.sql
```

Notes:
- The migration is forward-only and may lock the table while data is copied.
- If the table is already partitioned, `015` skips data migration and only ensures future partitions.
  (This guard prevents repeated heavy rewrites.)

Rollback (manual):

- Create a new non-partitioned `telemetry_points` table and copy data back.
- Drop the partitioned table and partitions.
- Recreate original indexes.

## Indexes

Created on the parent partitioned table (automatically applied to partitions):

- `idx_telemetry_points_station_ts` on `(tenant_id, station_id, ts)`
- `idx_telemetry_points_device_ts` on `(tenant_id, device_id, ts)`
- `idx_telemetry_points_station_point_ts` on `(tenant_id, station_id, point_key, ts DESC)`
- `idx_telemetry_points_station_device_point_ts` on `(tenant_id, station_id, device_id, point_key, ts DESC)`

Index health check (examples):

```sql
SELECT relname AS table_name, indexrelname AS index_name, idx_scan, idx_tup_read
FROM pg_stat_user_indexes
WHERE relname LIKE 'telemetry_points%'
ORDER BY idx_scan DESC;
```

```sql
SELECT tablename, indexname, indexdef
FROM pg_indexes
WHERE tablename LIKE 'telemetry_points%'
ORDER BY tablename, indexname;
```

## Maintenance Script

`./scripts/pg_partition_maintain.sh`:

- Creates partitions from the current month through `FUTURE_MONTHS` (default 3).
- Detaches and drops partitions with end time earlier than `RETENTION_DAYS`.
- Supports dry-run.
- If the table is still day-partitioned, the script will fall back to `FUTURE_DAYS`.

Examples:

```bash
DRY_RUN=1 RETENTION_DAYS=90 FUTURE_MONTHS=3 ./scripts/pg_partition_maintain.sh
```

```bash
RETENTION_DAYS=120 FUTURE_MONTHS=6 ./scripts/pg_partition_maintain.sh
```

## Scaling Notes

- If ingestion rate grows, consider:
  - More frequent partition maintenance (e.g., daily cron).
  - Adjusting `FUTURE_MONTHS` to avoid runtime partition creation.
  - Increasing autovacuum on partitions with heavy churn.

## Operational Notes

- The default partition should remain mostly empty; if it grows, run the maintenance script to create missing partitions and backfill as needed.
- For archiving, replace `DROP TABLE` with a `DETACH PARTITION` + `pg_dump` (TODO in script).
