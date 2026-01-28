package eventing

import (
	"encoding/json"
	"errors"
	"reflect"
	"sync"
)

// Registry maps event type names to constructors for decoding payloads.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]func() any
}

// NewRegistry constructs a registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]func() any)}
}

// Register registers an event type (value or pointer).
func (r *Registry) Register(sample any) {
	if r == nil || sample == nil {
		return
	}
	t := reflect.TypeOf(sample)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	name := t.String()
	r.mu.Lock()
	r.factories[name] = func() any {
		return reflect.New(t).Interface()
	}
	r.mu.Unlock()
}

// DecodePayload decodes envelope payload into a concrete event.
func (r *Registry) DecodePayload(env Envelope) (any, error) {
	if r == nil {
		return nil, errors.New("eventing: nil registry")
	}
	r.mu.RLock()
	factory := r.factories[env.EventType]
	r.mu.RUnlock()
	if factory == nil {
		return nil, errors.New("eventing: unknown event type")
	}
	target := factory()
	if err := json.Unmarshal(env.Payload, target); err != nil {
		return nil, err
	}
	value := reflect.ValueOf(target)
	if value.Kind() == reflect.Ptr && !value.IsNil() {
		return value.Elem().Interface(), nil
	}
	return target, nil
}
