# M4 Eventing (Envelope / Idempotency / DLQ / Outbox)

This document describes the production-grade eventing conventions used in the service.

## 1) Event Envelope

All events are wrapped with a common envelope before being written to outbox or DLQ.

Envelope fields:
- `event_id` (string) — unique id for the event
- `occurred_at` (RFC3339 UTC)
- `correlation_id` (string) — defaults to `event_id` if not provided
- `tenant_id` (string)
- `station_id` (string)
- `schema_version` (int, default `1`)
- `payload` (JSON) — original event payload

Covered event payloads:
- `TelemetryWindowClosed`
- `StatisticCalculated`
- `SettlementCalculated`

## 2) Outbox

Table: `event_outbox`

Events are **first persisted** to outbox after the business write succeeds.  
Dispatcher reads `status='pending'` rows, publishes to the in-process bus, then marks them `sent`.

Fields:
- `id`, `event_id`, `event_type`, `payload`, `status`, `attempts`, `created_at`, `sent_at`

Status values:
- `pending` → ready for dispatch
- `sent` → delivered successfully
- `failed` → dispatch failed (see DLQ)

## 3) Consumer Idempotency

Table: `processed_events`  
Primary key: `(event_id, consumer_name)`

Before handling an event, consumers check if `(event_id, consumer_name)` exists.
If yes, the handler is skipped. After successful handling, the record is inserted.

## 4) Dead Letter Queue (DLQ)

Table: `dead_letter_events`

When a handler returns an error, the dispatcher:
1. Marks the outbox record as `failed`
2. Inserts/updates a DLQ record with incremented `attempts`

Fields:
- `event_id`, `event_type`, `payload`, `error`, `first_seen_at`, `last_seen_at`, `attempts`

Failures do **not** block subsequent events.

## 5) Replay

You can replay by:
1. Fixing the issue
2. Re-inserting a DLQ record into outbox with `status='pending'`

Example SQL:

```sql
INSERT INTO event_outbox (id, event_id, event_type, payload, status, attempts)
SELECT event_id || '-replay', event_id, event_type, payload, 'pending', 0
FROM dead_letter_events
WHERE event_id = '...';
```

## 6) Correlation IDs

If present in context (`eventing.WithCorrelationID`), it will be stored in the envelope.  
Otherwise it defaults to the `event_id`.
