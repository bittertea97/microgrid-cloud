package eventing

import (
	"context"
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

// NewDispatcher constructs a dispatcher.
func NewDispatcher(bus EventBus, outbox OutboxStore, registry *Registry, dlq DLQStore) *Dispatcher {
	return &Dispatcher{bus: bus, outbox: outbox, registry: registry, dlq: dlq}
}

// Dispatch pulls pending outbox messages and delivers them.
func (d *Dispatcher) Dispatch(ctx context.Context, limit int) error {
	if d == nil || d.outbox == nil || d.bus == nil || d.registry == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	records, err := d.outbox.ListPending(ctx, limit)
	if err != nil {
		return err
	}

	for _, record := range records {
		env := record.Envelope
		payload, err := d.registry.DecodePayload(env)
		if err != nil {
			_ = d.outbox.MarkFailed(ctx, record.ID)
			if d.dlq != nil {
				_ = d.dlq.RecordFailure(ctx, env, err)
			}
			continue
		}

		ctxWithEnv := WithEnvelope(ctx, env)
		if err := d.bus.Publish(ctxWithEnv, payload); err != nil {
			_ = d.outbox.MarkFailed(ctx, record.ID)
			if d.dlq != nil {
				_ = d.dlq.RecordFailure(ctx, env, err)
			}
			continue
		}

		_ = d.outbox.MarkSent(ctx, record.ID)
	}
	return nil
}
