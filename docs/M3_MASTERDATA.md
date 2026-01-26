# M3 MasterData + PointMapping

This document explains the minimal masterdata model used to map raw telemetry `point_key` to stable business semantics used by analytics.

All times are stored in UTC. Telemetry still writes to `telemetry_points`, but analytics now **requires** point mappings to resolve semantics.

## Tables

### stations
Columns:
- `id`
- `tenant_id`
- `name`
- `timezone`
- `station_type`
- `region`
- `created_at`
- `updated_at`

### devices
Columns:
- `id`
- `station_id`
- `tb_entity_id` (mapping only; no TB domain concept)
- `device_type`
- `name`
- `created_at`
- `updated_at`

### point_mappings
Columns:
- `id`
- `station_id`
- `device_id` (nullable)
- `point_key`
- `semantic`
- `unit`
- `factor`
- `created_at`
- `updated_at`

## Semantics (minimal set for analytics)

- `charge_power_kw`
- `discharge_power_kw`
- `earnings`
- `carbon_reduction`

Analytics ignores telemetry points without a mapping. If **no mappings exist for a station**, hourly statistics will return an error (and skip calculation).

## Example: seed a station and mappings

```sql
INSERT INTO stations (id, tenant_id, name, timezone, station_type, region)
VALUES ('station-demo-001', 'tenant-demo', 'demo-station', 'UTC', 'microgrid', 'lab');

INSERT INTO point_mappings (id, station_id, point_key, semantic, unit, factor) VALUES
  ('station-demo-001-map-charge', 'station-demo-001', 'charge_power_kw', 'charge_power_kw', 'kW', 1),
  ('station-demo-001-map-discharge', 'station-demo-001', 'discharge_power_kw', 'discharge_power_kw', 'kW', 1),
  ('station-demo-001-map-earnings', 'station-demo-001', 'earnings', 'earnings', 'CNY', 1),
  ('station-demo-001-map-carbon', 'station-demo-001', 'carbon_reduction', 'carbon_reduction', 'kg', 1);
```

## Factor usage

`factor` is applied when resolving telemetry values:

```
value_semantic = value_raw * factor
```

Example:

```sql
INSERT INTO point_mappings (id, station_id, point_key, semantic, unit, factor)
VALUES ('map-charge-2x', 'station-demo-001', 'tb_charge', 'charge_power_kw', 'kW', 2.0);
```

If a telemetry measurement arrives with `point_key='tb_charge'` and `value_numeric=1.0`, the analytics pipeline uses `2.0`.

## Device-specific mappings (future)

`device_id` allows per-device mappings. The current analytics pipeline only uses **station-level mappings** (`device_id IS NULL`). Device-scoped mappings will be applied in a later extension once telemetry queries include device context.
