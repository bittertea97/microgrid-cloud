-- 015_telemetry_partition.sql

-- NOTE:
-- This migration is forward-only. It rewrites telemetry_points into a monthly-partitioned table and copies data.
-- Rollback requires manual export + restore of telemetry_points (see docs/PG_RETENTION.md).

BEGIN;

SET TIME ZONE 'UTC';

DO $$
DECLARE
	table_exists boolean;
	is_partitioned boolean;
	has_children boolean;
	already_monthly boolean := false;
	min_ts timestamptz;
	max_ts timestamptz;
	start_month date;
	end_month date;
	future_months int := 3;
	future_days int := 30;
	d date;
	sample_bound text;
	start_text text;
	end_text text;
	start_ts timestamptz;
	end_ts timestamptz;
	mode text := 'month';
BEGIN
	SELECT EXISTS (
		SELECT 1
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'telemetry_points'
	) INTO table_exists;

	IF NOT table_exists THEN
		RAISE NOTICE 'telemetry_points missing; skip';
		RETURN;
	END IF;

	SELECT EXISTS (
		SELECT 1
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
			AND c.relname = 'telemetry_points'
			AND c.relkind = 'p'
	) INTO is_partitioned;

	SELECT EXISTS (
		SELECT 1
		FROM pg_inherits i
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = 'telemetry_points'
	) INTO has_children;

	IF is_partitioned OR has_children THEN
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
					already_monthly := true;
					mode := 'month';
				ELSIF end_ts - start_ts = interval '1 day' THEN
					mode := 'day';
				END IF;
			END IF;
		END IF;
	END IF;

	IF is_partitioned OR has_children THEN
		RAISE NOTICE 'telemetry_points already partitioned; skip data migration';
		EXECUTE 'CREATE INDEX IF NOT EXISTS idx_telemetry_points_station_ts ON telemetry_points (tenant_id, station_id, ts)';
		EXECUTE 'CREATE INDEX IF NOT EXISTS idx_telemetry_points_device_ts ON telemetry_points (tenant_id, device_id, ts)';
		EXECUTE 'CREATE INDEX IF NOT EXISTS idx_telemetry_points_station_point_ts ON telemetry_points (tenant_id, station_id, point_key, ts DESC)';
		EXECUTE 'CREATE INDEX IF NOT EXISTS idx_telemetry_points_station_device_point_ts ON telemetry_points (tenant_id, station_id, device_id, point_key, ts DESC)';

		IF mode = 'day' THEN
			FOR d IN SELECT generate_series(current_date, current_date + future_days, interval ''1 day'')::date LOOP
				EXECUTE format(
					'CREATE TABLE IF NOT EXISTS telemetry_points_%s PARTITION OF telemetry_points FOR VALUES FROM (%L) TO (%L)',
					to_char(d, 'YYYYMMDD'),
					d::timestamptz,
					(d + 1)::timestamptz
				);
			END LOOP;
		ELSE
			FOR d IN SELECT generate_series(date_trunc('month', current_date)::date, (date_trunc('month', current_date) + make_interval(months => future_months))::date, interval ''1 month'')::date LOOP
				EXECUTE format(
					'CREATE TABLE IF NOT EXISTS telemetry_points_%s PARTITION OF telemetry_points FOR VALUES FROM (%L) TO (%L)',
					to_char(d, 'YYYYMM'),
					d::timestamptz,
					(d + interval ''1 month'')::timestamptz
				);
			END LOOP;
		END IF;
		RETURN;
	END IF;

	EXECUTE 'ALTER TABLE telemetry_points RENAME TO telemetry_points_legacy';

	IF EXISTS (SELECT 1 FROM pg_class WHERE relname = ''telemetry_points_default'') THEN
		EXECUTE 'ALTER TABLE telemetry_points_default RENAME TO telemetry_points_legacy_default';
	END IF;

	IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = ''telemetry_points_pkey'') THEN
		EXECUTE 'ALTER TABLE telemetry_points_legacy RENAME CONSTRAINT telemetry_points_pkey TO telemetry_points_legacy_pkey';
	END IF;

	IF EXISTS (SELECT 1 FROM pg_class WHERE relname = ''idx_telemetry_points_station_ts'') THEN
		EXECUTE 'ALTER INDEX idx_telemetry_points_station_ts RENAME TO idx_telemetry_points_station_ts_legacy';
	END IF;

	IF EXISTS (SELECT 1 FROM pg_class WHERE relname = ''idx_telemetry_points_device_ts'') THEN
		EXECUTE 'ALTER INDEX idx_telemetry_points_device_ts RENAME TO idx_telemetry_points_device_ts_legacy';
	END IF;

	IF EXISTS (SELECT 1 FROM pg_class WHERE relname = ''idx_telemetry_points_station_point_ts'') THEN
		EXECUTE 'ALTER INDEX idx_telemetry_points_station_point_ts RENAME TO idx_telemetry_points_station_point_ts_legacy';
	END IF;

	IF EXISTS (SELECT 1 FROM pg_class WHERE relname = ''idx_telemetry_points_station_device_point_ts'') THEN
		EXECUTE 'ALTER INDEX idx_telemetry_points_station_device_point_ts RENAME TO idx_telemetry_points_station_device_point_ts_legacy';
	END IF;

	EXECUTE '
CREATE TABLE telemetry_points (
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	point_key TEXT NOT NULL,
	ts TIMESTAMPTZ NOT NULL,
	value_numeric DOUBLE PRECISION,
	value_text TEXT,
	quality TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT telemetry_points_pkey PRIMARY KEY (tenant_id, station_id, device_id, point_key, ts)
) PARTITION BY RANGE (ts)';

	EXECUTE 'CREATE TABLE IF NOT EXISTS telemetry_points_default PARTITION OF telemetry_points DEFAULT';

	SELECT min(ts), max(ts) INTO min_ts, max_ts FROM telemetry_points_legacy;
	IF min_ts IS NULL OR max_ts IS NULL THEN
		start_month := date_trunc('month', current_date)::date;
		end_month := (start_month + make_interval(months => future_months))::date;
	ELSE
		start_month := date_trunc('month', min_ts)::date;
		end_month := (date_trunc('month', max_ts) + make_interval(months => future_months))::date;
	END IF;

	FOR d IN SELECT generate_series(start_month, end_month, interval ''1 month'')::date LOOP
		EXECUTE format(
			'CREATE TABLE IF NOT EXISTS telemetry_points_%s PARTITION OF telemetry_points FOR VALUES FROM (%L) TO (%L)',
			to_char(d, 'YYYYMM'),
			d::timestamptz,
			(d + interval ''1 month'')::timestamptz
		);
	END LOOP;

	EXECUTE '
INSERT INTO telemetry_points (
	tenant_id,
	station_id,
	device_id,
	point_key,
	ts,
	value_numeric,
	value_text,
	quality,
	created_at,
	updated_at
)
SELECT
	tenant_id,
	station_id,
	device_id,
	point_key,
	ts,
	value_numeric,
	value_text,
	quality,
	created_at,
	updated_at
FROM telemetry_points_legacy';

	EXECUTE 'DROP TABLE telemetry_points_legacy';

	EXECUTE 'CREATE INDEX IF NOT EXISTS idx_telemetry_points_station_ts ON telemetry_points (tenant_id, station_id, ts)';
	EXECUTE 'CREATE INDEX IF NOT EXISTS idx_telemetry_points_device_ts ON telemetry_points (tenant_id, device_id, ts)';
	EXECUTE 'CREATE INDEX IF NOT EXISTS idx_telemetry_points_station_point_ts ON telemetry_points (tenant_id, station_id, point_key, ts DESC)';
	EXECUTE 'CREATE INDEX IF NOT EXISTS idx_telemetry_points_station_device_point_ts ON telemetry_points (tenant_id, station_id, device_id, point_key, ts DESC)';
END $$;

COMMIT;
