-- 010_telemetry_partition.sql (legacy daily partition) [DEPRECATED]
--
-- NOTE:
-- This legacy migration partitions telemetry_points by day. It is superseded by 015_telemetry_partition.sql
-- which converts to monthly partitions. Forward-only; rollback requires manual export + restore.

BEGIN;

SET TIME ZONE 'UTC';

DO $$
DECLARE
	is_partitioned boolean;
	min_ts timestamptz;
	max_ts timestamptz;
	start_date date;
	end_date date;
	future_days int := 30;
	d date;
BEGIN
	SELECT EXISTS (
		SELECT 1
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
			AND c.relname = 'telemetry_points'
			AND c.relkind = 'p'
	) INTO is_partitioned;

	IF is_partitioned THEN
		RAISE NOTICE 'telemetry_points already partitioned';
		RETURN;
	END IF;

	EXECUTE 'ALTER TABLE telemetry_points RENAME TO telemetry_points_legacy';
	EXECUTE 'DROP INDEX IF EXISTS idx_telemetry_points_station_ts';

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
	PRIMARY KEY (tenant_id, station_id, device_id, point_key, ts)
) PARTITION BY RANGE (ts)';

	EXECUTE 'CREATE TABLE IF NOT EXISTS telemetry_points_default PARTITION OF telemetry_points DEFAULT';

	SELECT min(ts), max(ts) INTO min_ts, max_ts FROM telemetry_points_legacy;
	IF min_ts IS NULL OR max_ts IS NULL THEN
		start_date := current_date - 7;
		end_date := current_date + future_days;
	ELSE
		start_date := min_ts::date;
		end_date := max_ts::date + future_days;
	END IF;

	FOR d IN SELECT generate_series(start_date, end_date, interval '1 day')::date LOOP
		EXECUTE format(
			'CREATE TABLE IF NOT EXISTS telemetry_points_%s PARTITION OF telemetry_points FOR VALUES FROM (%L) TO (%L)',
			to_char(d, 'YYYYMMDD'),
			d::timestamptz,
			(d + 1)::timestamptz
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
