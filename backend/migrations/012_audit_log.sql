-- 012_audit_log.sql

CREATE TABLE IF NOT EXISTS audit_logs (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	actor TEXT NOT NULL,
	role TEXT NOT NULL,
	action TEXT NOT NULL,
	resource_type TEXT NOT NULL,
	resource_id TEXT NOT NULL,
	station_id TEXT,
	metadata JSONB,
	ip TEXT,
	user_agent TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_time
	ON audit_logs (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_station_time
	ON audit_logs (station_id, created_at DESC);
