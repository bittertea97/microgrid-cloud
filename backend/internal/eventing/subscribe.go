package eventing

import (
	"context"
	"reflect"
	"time"

	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/observability/metrics"
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
		observeConsumerLag(ctx, event, consumerName)
		if err := handler(ctx, event); err != nil {
			return err
		}
		return store.MarkProcessed(ctx, env.EventID, consumerName)
	}
}

func observeConsumerLag(ctx context.Context, event any, consumerName string) {
	occurredAt := time.Time{}
	if env, ok := EnvelopeFromContext(ctx); ok {
		occurredAt = env.OccurredAt
	}
	if occurredAt.IsZero() {
		occurredAt = extractOccurredAt(event)
	}
	if occurredAt.IsZero() {
		return
	}
	metrics.ObserveConsumerLag(consumerName, time.Since(occurredAt))
}

func extractOccurredAt(event any) time.Time {
	if event == nil {
		return time.Time{}
	}
	value := reflect.ValueOf(event)
	for value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return time.Time{}
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return time.Time{}
	}
	field := value.FieldByName("OccurredAt")
	if !field.IsValid() {
		return time.Time{}
	}
	if t, ok := field.Interface().(time.Time); ok {
		return t
	}
	return time.Time{}
}
