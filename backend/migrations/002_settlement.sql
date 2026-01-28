-- 002_settlement.sql

CREATE TABLE IF NOT EXISTS settlements_day (
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	day_start TIMESTAMPTZ NOT NULL,
	energy_kwh DOUBLE PRECISION NOT NULL DEFAULT 0,
	amount DOUBLE PRECISION NOT NULL DEFAULT 0,
	currency TEXT NOT NULL DEFAULT 'CNY',
	status TEXT NOT NULL DEFAULT 'CALCULATED',
	version INTEGER NOT NULL DEFAULT 1,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (tenant_id, station_id, day_start)
);

CREATE INDEX IF NOT EXISTS idx_settlements_day_station
	ON settlements_day (tenant_id, station_id, day_start);
