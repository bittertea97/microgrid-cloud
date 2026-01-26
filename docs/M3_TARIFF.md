# M3 Tariff Plan (fixed / TOU)

This document explains how to configure tariff plans and rules for settlement pricing.

## Tables

### tariff_plans
Columns:
- `id`
- `tenant_id`
- `station_id`
- `effective_month` (DATE, first day of month in UTC)
- `currency`
- `mode` (`fixed` or `tou`)
- `created_at`
- `updated_at`

### tariff_rules
Columns:
- `id`
- `plan_id`
- `start_minute`
- `end_minute`
- `price_per_kwh`
- `created_at`
- `updated_at`

`start_minute` / `end_minute` define a `[start,end)` window within a day.  
Valid range: `0..1440`.

## Fixed price example

```sql
INSERT INTO tariff_plans (id, tenant_id, station_id, effective_month, currency, mode)
VALUES ('plan-fixed-202601', 'tenant-demo', 'station-demo-001', '2026-01-01', 'CNY', 'fixed');

INSERT INTO tariff_rules (id, plan_id, start_minute, end_minute, price_per_kwh)
VALUES ('rule-fixed-202601', 'plan-fixed-202601', 0, 1440, 1.20);
```

## TOU (time-of-use) example

```sql
INSERT INTO tariff_plans (id, tenant_id, station_id, effective_month, currency, mode)
VALUES ('plan-tou-202601', 'tenant-demo', 'station-demo-001', '2026-01-01', 'CNY', 'tou');

INSERT INTO tariff_rules (id, plan_id, start_minute, end_minute, price_per_kwh) VALUES
  ('rule-offpeak', 'plan-tou-202601', 0,   480, 0.50),   -- 00:00-08:00
  ('rule-peak',    'plan-tou-202601', 480, 1080, 1.50), -- 08:00-18:00
  ('rule-mid',     'plan-tou-202601', 1080, 1440, 0.80);-- 18:00-24:00
```

## Settlement logic

For a given day:
1. Load 24 hour statistics (`analytics_statistics` with `time_type='HOUR'`).
2. For each hour start `ts`, find the matching `tariff_rules` by minute-of-day and get `price_per_kwh`.
3. `amount_day = Σ(energy_hour * price_hour)`

If the tariff plan or rule is missing for a given hour, settlement fails with an error.
