# Analytics Event Contracts

This document defines the event model for the Analytics context only. The goal is to keep events immutable, replayable, and decoupled from other contexts.

## Event envelope (common fields)
- event_id: Unique event id (idempotency key).
- event_type: Event name. See naming convention below.
- event_version: Integer version, starts at 1. Bump on breaking change.
- occurred_at: Business time when the event fact happened (UTC).
- produced_at: Time when the event was published (UTC).
- tenant_id: Tenant isolation.
- aggregate_id: Analytics subject identifier (for example: site:SH-001, meter:MTR-01).
- correlation_id: Trace across a business flow.
- causation_id: Upstream event id that caused this event.
- is_replay: True when emitted by backfill/replay.
- replay_id: Replay batch id (empty when is_replay is false).
- payload: Event specific data.
- meta: Open metadata map for non-contractual hints.

Time semantics
- All timestamps are RFC3339 in UTC.
- Time windows use [start, end) semantics: start inclusive, end exclusive.
- occurred_at expresses business reality; produced_at expresses transport time.

## Event: TelemetryWindowClosed
Business meaning: telemetry for a given hour window is closed and can be converted into an hourly statistic.

Payload fields
- window_start: Start of telemetry window (UTC, inclusive).
- window_end: End of telemetry window (UTC, exclusive).
- source: Optional source tag (tsdb, edge, etc).
- quality: Data quality summary (complete, partial, late).
- checksum: Optional checksum of source data for replay consistency checks.

Notes
- This event does not carry settlement or pricing data.
- This event does not include any raw telemetry values.

## Event: StatisticCalculated
Business meaning: a statistic fact for a window is computed and frozen.

Payload fields
- statistic_id: Unique statistic fact id (immutable).
- window_start: Window start (UTC, inclusive).
- window_end: Window end (UTC, exclusive).
- granularity: hour | day | month | year.
- metrics: Map of statistic values with unit.
- frozen_at: Time when the statistic became immutable (UTC).
- derivation: Source layer (telemetry, hour, day, month).
- input_facts: Optional list/hash of input statistic ids.

Notes
- Hour is the only source of truth. Day/month/year are rollups.
- A replay produces a new statistic_id; old facts remain immutable.

## Replay and backfill
- Consumers must handle duplicate delivery by event_id.
- Replay events set is_replay=true and include replay_id.
- Backfill should be traceable via correlation_id and causation_id.

## Naming convention
- Format: <context>.<noun>.<past_tense_verb>
- Lowercase, dot separated.
- Do not embed version in event_type; use event_version.

Examples
- analytics.telemetry_window.closed
- analytics.statistic.calculated