-- 009_alarms.sql

CREATE TABLE IF NOT EXISTS alarm_rules (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	name TEXT NOT NULL,
	semantic TEXT NOT NULL,
	operator TEXT NOT NULL,
	threshold DOUBLE PRECISION NOT NULL,
	hysteresis DOUBLE PRECISION NOT NULL DEFAULT 0,
	duration_seconds INTEGER NOT NULL DEFAULT 0,
	severity TEXT NOT NULL DEFAULT 'medium',
	enabled BOOLEAN NOT NULL DEFAULT TRUE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alarm_rules_station
	ON alarm_rules (tenant_id, station_id);

CREATE INDEX IF NOT EXISTS idx_alarm_rules_semantic
	ON alarm_rules (station_id, semantic);

CREATE TABLE IF NOT EXISTS alarms (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	originator_type TEXT NOT NULL,
	originator_id TEXT NOT NULL,
	rule_id TEXT NOT NULL REFERENCES alarm_rules(id),
	status TEXT NOT NULL,
	start_at TIMESTAMPTZ NOT NULL,
	end_at TIMESTAMPTZ,
	last_value DOUBLE PRECISION,
	acked_at TIMESTAMPTZ,
	cleared_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alarms_station_time
	ON alarms (tenant_id, station_id, start_at);

CREATE INDEX IF NOT EXISTS idx_alarms_status
	ON alarms (status, updated_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_active_alarm
	ON alarms (tenant_id, rule_id, originator_type, originator_id)
	WHERE status IN ('active', 'acknowledged');

CREATE TABLE IF NOT EXISTS alarm_rule_states (
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	rule_id TEXT NOT NULL,
	originator_type TEXT NOT NULL,
	originator_id TEXT NOT NULL,
	pending_since TIMESTAMPTZ NOT NULL,
	last_value DOUBLE PRECISION,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (tenant_id, rule_id, originator_type, originator_id)
);

CREATE INDEX IF NOT EXISTS idx_alarm_rule_states_station
	ON alarm_rule_states (tenant_id, station_id);
