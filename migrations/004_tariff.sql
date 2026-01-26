-- 004_tariff.sql

CREATE TABLE IF NOT EXISTS tariff_plans (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	station_id TEXT NOT NULL,
	effective_month DATE NOT NULL,
	currency TEXT NOT NULL DEFAULT 'CNY',
	mode TEXT NOT NULL DEFAULT 'fixed',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tariff_plans_station_month
	ON tariff_plans (tenant_id, station_id, effective_month);

CREATE TABLE IF NOT EXISTS tariff_rules (
	id TEXT PRIMARY KEY,
	plan_id TEXT NOT NULL REFERENCES tariff_plans(id),
	start_minute INTEGER NOT NULL,
	end_minute INTEGER NOT NULL,
	price_per_kwh DOUBLE PRECISION NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tariff_rules_plan
	ON tariff_rules (plan_id, start_minute, end_minute);
