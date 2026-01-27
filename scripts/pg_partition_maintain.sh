#!/usr/bin/env bash
set -euo pipefail

DSN="${PG_DSN:-${DATABASE_URL:-}}"
if [[ -z "${DSN}" ]]; then
  echo "PG_DSN or DATABASE_URL is required" >&2
  exit 1
fi

RETENTION_DAYS="${RETENTION_DAYS:-90}"
FUTURE_MONTHS="${FUTURE_MONTHS:-3}"
FUTURE_DAYS="${FUTURE_DAYS:-30}"
DRY_RUN="${DRY_RUN:-0}"

read -r -d '' SQL <<'SQL'
SET TIME ZONE 'UTC';

DO $$
DECLARE
  future_months int := current_setting('telemetry.future_months')::int;
  future_days int := current_setting('telemetry.future_days')::int;
  retention_days int := current_setting('telemetry.retention_days')::int;
  cutoff timestamptz := now() - make_interval(days => retention_days);
  d date;
  part record;
  start_ts timestamptz;
  end_ts timestamptz;
  start_text text;
  end_text text;
  sample_bound text;
  mode text := 'month';
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relname = 'telemetry_points' AND c.relkind = 'p'
  ) THEN
    RAISE NOTICE 'telemetry_points is not partitioned; skip maintenance';
    RETURN;
  END IF;

  SELECT pg_get_expr(c.relpartbound, c.oid)
  INTO sample_bound
  FROM pg_inherits i
  JOIN pg_class c ON c.oid = i.inhrelid
  JOIN pg_class p ON p.oid = i.inhparent
  WHERE p.relname = 'telemetry_points'
    AND c.relname <> 'telemetry_points_default'
  LIMIT 1;

  IF sample_bound IS NOT NULL THEN
    start_text := substring(sample_bound FROM 'FROM \(''([^'']+)''\)');
    end_text := substring(sample_bound FROM 'TO \(''([^'']+)''\)');
    IF start_text IS NOT NULL AND end_text IS NOT NULL THEN
      start_ts := start_text::timestamptz;
      end_ts := end_text::timestamptz;
      IF date_trunc('month', start_ts) + interval '1 month' = end_ts THEN
        mode := 'month';
      ELSIF end_ts - start_ts = interval '1 day' THEN
        mode := 'day';
      END IF;
    END IF;
  END IF;

  IF mode = 'day' THEN
    FOR d IN SELECT generate_series(current_date, current_date + future_days, interval '1 day')::date LOOP
      EXECUTE format(
        'CREATE TABLE IF NOT EXISTS telemetry_points_%s PARTITION OF telemetry_points FOR VALUES FROM (%L) TO (%L)',
        to_char(d, 'YYYYMMDD'),
        d::timestamptz,
        (d + 1)::timestamptz
      );
    END LOOP;
  ELSE
    FOR d IN SELECT generate_series(date_trunc('month', current_date)::date, (date_trunc('month', current_date) + make_interval(months => future_months))::date, interval '1 month')::date LOOP
      EXECUTE format(
        'CREATE TABLE IF NOT EXISTS telemetry_points_%s PARTITION OF telemetry_points FOR VALUES FROM (%L) TO (%L)',
        to_char(d, 'YYYYMM'),
        d::timestamptz,
        (d + interval ''1 month'')::timestamptz
      );
    END LOOP;
  END IF;

  FOR part IN
    SELECT c.relname, pg_get_expr(c.relpartbound, c.oid) AS bound
    FROM pg_inherits i
    JOIN pg_class c ON c.oid = i.inhrelid
    JOIN pg_class p ON p.oid = i.inhparent
    WHERE p.relname = 'telemetry_points'
  LOOP
    IF part.bound = 'DEFAULT' THEN
      CONTINUE;
    END IF;

    start_text := substring(part.bound FROM 'FROM \(''([^'']+)''\)');
    end_text := substring(part.bound FROM 'TO \(''([^'']+)''\)');
    IF start_text IS NULL OR end_text IS NULL THEN
      CONTINUE;
    END IF;

    start_ts := start_text::timestamptz;
    end_ts := end_text::timestamptz;

    IF end_ts < cutoff THEN
      EXECUTE format('ALTER TABLE telemetry_points DETACH PARTITION %I', part.relname);
      -- TODO: archive detached partition before dropping.
      EXECUTE format('DROP TABLE IF EXISTS %I', part.relname);
    END IF;
  END LOOP;
END $$;
SQL

if [[ "${DRY_RUN}" == "1" ]]; then
  echo "-- DRY RUN: no changes applied"
  echo "SET telemetry.future_months = ${FUTURE_MONTHS};"
  echo "SET telemetry.future_days = ${FUTURE_DAYS};"
  echo "SET telemetry.retention_days = ${RETENTION_DAYS};"
  echo "${SQL}"
  exit 0
fi

psql "${DSN}" -v ON_ERROR_STOP=1 \
  -c "SET telemetry.future_months = ${FUTURE_MONTHS};" \
  -c "SET telemetry.future_days = ${FUTURE_DAYS};" \
  -c "SET telemetry.retention_days = ${RETENTION_DAYS};" \
  -c "${SQL}"
