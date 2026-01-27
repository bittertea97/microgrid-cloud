-- 008_statements.sql

CREATE TABLE IF NOT EXISTS settlement_statements (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	statement_month DATE NOT NULL,
	category TEXT NOT NULL DEFAULT 'owner',
	status TEXT NOT NULL DEFAULT 'draft',
	version INTEGER NOT NULL DEFAULT 1,
	total_energy_kwh DOUBLE PRECISION NOT NULL DEFAULT 0,
	total_amount DOUBLE PRECISION NOT NULL DEFAULT 0,
	currency TEXT NOT NULL DEFAULT 'CNY',
	snapshot_hash TEXT,
	void_reason TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	frozen_at TIMESTAMPTZ,
	voided_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_settlement_statements_version
	ON settlement_statements (tenant_id, station_id, statement_month, category, version);

CREATE TABLE IF NOT EXISTS settlement_statement_items (
	statement_id TEXT NOT NULL REFERENCES settlement_statements(id),
	day_start TIMESTAMPTZ NOT NULL,
	energy_kwh DOUBLE PRECISION NOT NULL DEFAULT 0,
	amount DOUBLE PRECISION NOT NULL DEFAULT 0,
	currency TEXT NOT NULL DEFAULT 'CNY',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (statement_id, day_start)
);

CREATE TABLE IF NOT EXISTS statement_exports (
	id TEXT PRIMARY KEY,
	statement_id TEXT NOT NULL REFERENCES settlement_statements(id),
	format TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'generated',
	path_or_key TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
