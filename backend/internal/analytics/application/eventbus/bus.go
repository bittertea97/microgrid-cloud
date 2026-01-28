package eventbus

import (
	"context"
	"errors"
	"reflect"
	"sync"
)

// EventHandler handles a published event.
type EventHandler func(ctx context.Context, event any) error

// EventBus delivers events to subscribed handlers.
type EventBus interface {
	Publish(ctx context.Context, event any) error
	Subscribe(eventType string, handler EventHandler)
}

// ErrNilEvent is returned when a nil event is published.
var ErrNilEvent = errors.New("eventbus: nil event")

// ErrInvalidEventType is returned when the event type cannot be determined.
var ErrInvalidEventType = errors.New("eventbus: invalid event type")

// InMemoryBus is a minimal in-process event bus.
type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]EventHandler
}

// NewInMemoryBus constructs a new in-memory bus.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		handlers: make(map[string][]EventHandler),
	}
}

// Publish dispatches an event to all handlers of its type.
func (b *InMemoryBus) Publish(ctx context.Context, event any) error {
	if event == nil {
		return ErrNilEvent
	}

	eventType := EventType(event)
	if eventType == "" {
		return ErrInvalidEventType
	}

	b.mu.RLock()
	handlers := append([]EventHandler(nil), b.handlers[eventType]...)
	b.mu.RUnlock()

	var firstErr error
	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Subscribe registers a handler for an event type.
func (b *InMemoryBus) Subscribe(eventType string, handler EventHandler) {
	if eventType == "" || handler == nil {
		return
	}

	b.mu.Lock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
	b.mu.Unlock()
}

// EventType returns the fully-qualified type name for an event instance.
func EventType(event any) string {
	if event == nil {
		return ""
	}
	t := reflect.TypeOf(event)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.String()
}

// EventTypeOf returns the fully-qualified type name for a type parameter.
func EventTypeOf[T any]() string {
	return reflect.TypeOf((*T)(nil)).Elem().String()
}
