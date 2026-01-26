-- 003_masterdata.sql

CREATE TABLE IF NOT EXISTS stations (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	name TEXT NOT NULL,
	timezone TEXT NOT NULL DEFAULT 'UTC',
	station_type TEXT,
	region TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stations_tenant
	ON stations (tenant_id);

CREATE TABLE IF NOT EXISTS devices (
	id TEXT PRIMARY KEY,
	station_id TEXT NOT NULL REFERENCES stations(id),
	tb_entity_id TEXT,
	device_type TEXT,
	name TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_devices_station
	ON devices (station_id);

CREATE TABLE IF NOT EXISTS point_mappings (
	id TEXT PRIMARY KEY,
	station_id TEXT NOT NULL REFERENCES stations(id),
	device_id TEXT REFERENCES devices(id),
	point_key TEXT NOT NULL,
	semantic TEXT NOT NULL,
	unit TEXT NOT NULL,
	factor DOUBLE PRECISION NOT NULL DEFAULT 1,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_point_mappings_station
	ON point_mappings (station_id);
