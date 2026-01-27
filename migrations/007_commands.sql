-- 007_commands.sql

CREATE TABLE IF NOT EXISTS commands (
	command_id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	command_type TEXT NOT NULL,
	payload JSONB NOT NULL,
	idempotency_key TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'created',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	sent_at TIMESTAMPTZ,
	acked_at TIMESTAMPTZ,
	error TEXT
);

CREATE INDEX IF NOT EXISTS idx_commands_station_time
	ON commands (tenant_id, station_id, created_at);

CREATE INDEX IF NOT EXISTS idx_commands_idempotency
	ON commands (tenant_id, idempotency_key);
