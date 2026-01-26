package eventing

import (
	"context"

	"microgrid-cloud/internal/analytics/application/eventbus"
)

// ProcessedStore provides idempotency checks.
type ProcessedStore interface {
	HasProcessed(ctx context.Context, eventID, consumerName string) (bool, error)
	MarkProcessed(ctx context.Context, eventID, consumerName string) error
}

// Subscribe wraps handler with idempotency if store is provided.
func Subscribe(bus eventbus.EventBus, eventType, consumerName string, handler eventbus.EventHandler, store ProcessedStore) {
	if store == nil {
		bus.Subscribe(eventType, handler)
		return
	}
	bus.Subscribe(eventType, WrapHandler(consumerName, handler, store))
}

// WrapHandler enforces idempotency per consumer.
func WrapHandler(consumerName string, handler eventbus.EventHandler, store ProcessedStore) eventbus.EventHandler {
	return func(ctx context.Context, event any) error {
		env, ok := EnvelopeFromContext(ctx)
		if !ok || env.EventID == "" {
			return handler(ctx, event)
		}
		processed, err := store.HasProcessed(ctx, env.EventID, consumerName)
		if err != nil {
			return err
		}
		if processed {
			return nil
		}
		if err := handler(ctx, event); err != nil {
			return err
		}
		return store.MarkProcessed(ctx, env.EventID, consumerName)
	}
}
