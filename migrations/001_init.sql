-- 001_init.sql

CREATE TABLE IF NOT EXISTS analytics_statistics (
	subject_id TEXT NOT NULL,
	time_type TEXT NOT NULL,
	time_key TEXT NOT NULL,
	period_start TIMESTAMPTZ NOT NULL,
	statistic_id TEXT NOT NULL,
	is_completed BOOLEAN NOT NULL DEFAULT FALSE,
	completed_at TIMESTAMPTZ,
	charge_kwh DOUBLE PRECISION NOT NULL DEFAULT 0,
	discharge_kwh DOUBLE PRECISION NOT NULL DEFAULT 0,
	earnings DOUBLE PRECISION NOT NULL DEFAULT 0,
	carbon_reduction DOUBLE PRECISION NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (subject_id, time_type, time_key)
);

CREATE INDEX IF NOT EXISTS idx_analytics_statistics_period
	ON analytics_statistics (subject_id, time_type, period_start);

CREATE TABLE IF NOT EXISTS telemetry_points (
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
);

CREATE INDEX IF NOT EXISTS idx_telemetry_points_station_ts
	ON telemetry_points (tenant_id, station_id, ts);
