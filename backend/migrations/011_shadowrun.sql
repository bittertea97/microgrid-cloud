-- 011_shadowrun.sql

CREATE TABLE IF NOT EXISTS shadowrun_jobs (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	month DATE NOT NULL,
	job_date DATE NOT NULL,
	job_type TEXT NOT NULL DEFAULT 'shadowrun',
	status TEXT NOT NULL DEFAULT 'created',
	attempts INTEGER NOT NULL DEFAULT 0,
	error TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	started_at TIMESTAMPTZ,
	finished_at TIMESTAMPTZ,
	UNIQUE (tenant_id, station_id, month, job_date, job_type)
);

CREATE INDEX IF NOT EXISTS idx_shadowrun_jobs_station
	ON shadowrun_jobs (tenant_id, station_id, month, job_date);

CREATE TABLE IF NOT EXISTS shadowrun_reports (
	id TEXT PRIMARY KEY,
	job_id TEXT NOT NULL REFERENCES shadowrun_jobs(id),
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	month DATE NOT NULL,
	report_date DATE NOT NULL,
	status TEXT NOT NULL DEFAULT 'generated',
	report_location TEXT NOT NULL,
	diff_summary JSONB,
	diff_energy_kwh_max DOUBLE PRECISION NOT NULL DEFAULT 0,
	diff_amount_max DOUBLE PRECISION NOT NULL DEFAULT 0,
	missing_hours INTEGER NOT NULL DEFAULT 0,
	recommended_action TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shadowrun_reports_station
	ON shadowrun_reports (tenant_id, station_id, report_date);

CREATE TABLE IF NOT EXISTS system_alerts (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	category TEXT NOT NULL,
	severity TEXT NOT NULL,
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	payload JSONB,
	report_id TEXT,
	status TEXT NOT NULL DEFAULT 'open',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_alerts_station
	ON system_alerts (tenant_id, station_id, created_at);
