-- 013_strategy.sql

CREATE TABLE IF NOT EXISTS strategy_templates (
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL,
	params JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS strategies (
	station_id TEXT PRIMARY KEY,
	mode TEXT NOT NULL DEFAULT 'manual',
	enabled BOOLEAN NOT NULL DEFAULT FALSE,
	template_id TEXT NOT NULL REFERENCES strategy_templates(id),
	version INTEGER NOT NULL DEFAULT 1,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS strategy_calendar (
	strategy_id TEXT NOT NULL REFERENCES strategies(station_id),
	date DATE NOT NULL,
	enabled BOOLEAN NOT NULL DEFAULT TRUE,
	start_time TIME NOT NULL,
	end_time TIME NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (strategy_id, date)
);

CREATE INDEX IF NOT EXISTS idx_strategy_calendar_date
	ON strategy_calendar (strategy_id, date);

CREATE TABLE IF NOT EXISTS strategy_runs (
	strategy_id TEXT NOT NULL REFERENCES strategies(station_id),
	ts TIMESTAMPTZ NOT NULL,
	decision JSONB,
	command_id TEXT,
	status TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (strategy_id, ts)
);

CREATE INDEX IF NOT EXISTS idx_strategy_runs_ts
	ON strategy_runs (strategy_id, ts);
