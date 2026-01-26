-- 005_eventing.sql

CREATE TABLE IF NOT EXISTS processed_events (
	event_id TEXT NOT NULL,
	consumer_name TEXT NOT NULL,
	processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (event_id, consumer_name)
);

CREATE TABLE IF NOT EXISTS dead_letter_events (
	event_id TEXT PRIMARY KEY,
	event_type TEXT NOT NULL,
	payload JSONB NOT NULL,
	error TEXT NOT NULL,
	first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	attempts INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS event_outbox (
	id TEXT PRIMARY KEY,
	event_id TEXT NOT NULL,
	event_type TEXT NOT NULL,
	payload JSONB NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	attempts INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	sent_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_event_outbox_status
	ON event_outbox (status, created_at);
