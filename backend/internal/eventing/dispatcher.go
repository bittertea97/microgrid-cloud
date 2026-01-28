package eventing

import (
	"context"
	"time"

	"microgrid-cloud/internal/observability/metrics"
)

// Dispatcher sends outbox events to the in-process bus.
type Dispatcher struct {
	bus      EventBus
	outbox   OutboxStore
	registry *Registry
	dlq      DLQStore
}

// EventBus is the minimal publish interface.
type EventBus interface {
	Publish(ctx context.Context, event any) error
}

// OutboxStore provides access to outbox records.
type OutboxStore interface {
	ListPending(ctx context.Context, limit int) ([]OutboxRecord, error)
	MarkSent(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string) error
}

// DLQStore records failures.
type DLQStore interface {
	RecordFailure(ctx context.Context, env Envelope, err error) error
}

// OutboxRecord represents a pending outbox entry.
type OutboxRecord struct {
	ID       string
	Envelope Envelope
}

// DispatchResult captures the outcome of a dispatch run.
type DispatchResult struct {
	Requested int
	Claimed   int
	Sent      int
	Failed    int
	DLQ       int
}

// NewDispatcher constructs a dispatcher.
func NewDispatcher(bus EventBus, outbox OutboxStore, registry *Registry, dlq DLQStore) *Dispatcher {
	return &Dispatcher{bus: bus, outbox: outbox, registry: registry, dlq: dlq}
}

// Dispatch pulls pending outbox messages and delivers them.
func (d *Dispatcher) Dispatch(ctx context.Context, limit int) (DispatchResult, error) {
	start := time.Now()
	result := DispatchResult{Requested: limit}
	if d == nil || d.outbox == nil || d.bus == nil || d.registry == nil {
		metrics.ObserveOutboxDispatch(metrics.ResultError, time.Since(start), 0, 0, 0)
		return result, nil
	}
	if limit <= 0 {
		limit = 50
		result.Requested = limit
	}
	records, err := d.outbox.ListPending(ctx, limit)
	if err != nil {
		metrics.ObserveOutboxDispatch(metrics.ResultError, time.Since(start), 0, 0, 0)
		return result, err
	}
	result.Claimed = len(records)
	if result.Claimed == 0 {
		metrics.ObserveOutboxDispatch(metrics.ResultSuccess, time.Since(start), 0, 0, 0)
		return result, nil
	}
	var firstErr error

	for _, record := range records {
		env := record.Envelope
		payload, err := d.registry.DecodePayload(env)
		if err != nil {
			if err := d.outbox.MarkFailed(ctx, record.ID); err != nil && firstErr == nil {
				firstErr = err
			}
			if d.dlq != nil {
				if err := d.dlq.RecordFailure(ctx, env, err); err == nil {
					result.DLQ++
				}
			}
			result.Failed++
			continue
		}

		ctxWithEnv := WithEnvelope(ctx, env)
		if err := d.bus.Publish(ctxWithEnv, payload); err != nil {
			if err := d.outbox.MarkFailed(ctx, record.ID); err != nil && firstErr == nil {
				firstErr = err
			}
			if d.dlq != nil {
				if err := d.dlq.RecordFailure(ctx, env, err); err == nil {
					result.DLQ++
				}
			}
			result.Failed++
			continue
		}

		if err := d.outbox.MarkSent(ctx, record.ID); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			result.Failed++
			continue
		}
		result.Sent++
	}
	dispatchResult := metrics.ResultSuccess
	if firstErr != nil || result.Failed > 0 {
		dispatchResult = metrics.ResultError
	}
	metrics.ObserveOutboxDispatch(dispatchResult, time.Since(start), result.Sent, result.Failed, result.DLQ)
	return result, firstErr
}
