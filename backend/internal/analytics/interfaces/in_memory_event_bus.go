package interfaces

import (
	"context"
	"errors"
	"sync"

	"microgrid-cloud/internal/analytics/application/events"
)

// InMemoryEventBus is a lightweight in-process event bus for demos/tests.
type InMemoryEventBus struct {
	mu sync.RWMutex

	telemetryHandlers []func(context.Context, events.TelemetryWindowClosed) error
	hourHandlers      []func(context.Context, events.StatisticCalculated) error
	dayHandlers       []func(context.Context, events.StatisticCalculated) error
}

// NewInMemoryEventBus constructs a new bus.
func NewInMemoryEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{}
}

// SubscribeTelemetryWindowClosed registers a handler for TelemetryWindowClosed.
func (b *InMemoryEventBus) SubscribeTelemetryWindowClosed(handler func(context.Context, events.TelemetryWindowClosed) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.telemetryHandlers = append(b.telemetryHandlers, handler)
}

// PublishTelemetryWindowClosed publishes a TelemetryWindowClosed event.
func (b *InMemoryEventBus) PublishTelemetryWindowClosed(ctx context.Context, event events.TelemetryWindowClosed) error {
	b.mu.RLock()
	handlers := append([]func(context.Context, events.TelemetryWindowClosed) error(nil), b.telemetryHandlers...)
	b.mu.RUnlock()

	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// SubscribeHourStatisticCalculated registers a handler for hour statistic events.
func (b *InMemoryEventBus) SubscribeHourStatisticCalculated(handler func(context.Context, events.StatisticCalculated) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hourHandlers = append(b.hourHandlers, handler)
}

// PublishHourStatisticCalculated publishes a hour statistic calculated event.
func (b *InMemoryEventBus) PublishHourStatisticCalculated(ctx context.Context, event events.StatisticCalculated) error {
	b.mu.RLock()
	handlers := append([]func(context.Context, events.StatisticCalculated) error(nil), b.hourHandlers...)
	b.mu.RUnlock()

	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// SubscribeDayStatisticCalculated registers a handler for day statistic events.
func (b *InMemoryEventBus) SubscribeDayStatisticCalculated(handler func(context.Context, events.StatisticCalculated) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dayHandlers = append(b.dayHandlers, handler)
}

// PublishDayStatisticCalculated publishes a day statistic calculated event.
func (b *InMemoryEventBus) PublishDayStatisticCalculated(ctx context.Context, event events.StatisticCalculated) error {
	b.mu.RLock()
	handlers := append([]func(context.Context, events.StatisticCalculated) error(nil), b.dayHandlers...)
	b.mu.RUnlock()

	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		if err := handler(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// HourlyEventPublisher adapts the bus to application.EventPublisher.
type HourlyEventPublisher struct {
	bus *InMemoryEventBus
}

// NewHourlyEventPublisher constructs an adapter for hourly events.
func NewHourlyEventPublisher(bus *InMemoryEventBus) (*HourlyEventPublisher, error) {
	if bus == nil {
		return nil, errors.New("hourly event publisher: nil bus")
	}
	return &HourlyEventPublisher{bus: bus}, nil
}

// PublishStatisticCalculated publishes hour statistic events from application layer.
func (p *HourlyEventPublisher) PublishStatisticCalculated(ctx context.Context, event events.StatisticCalculated) error {
	return p.bus.PublishHourStatisticCalculated(ctx, event)
}

// DailyEventPublisher adapts the bus to StatisticCalculatedPublisher.
type DailyEventPublisher struct {
	bus *InMemoryEventBus
}

// NewDailyEventPublisher constructs an adapter for daily events.
func NewDailyEventPublisher(bus *InMemoryEventBus) (*DailyEventPublisher, error) {
	if bus == nil {
		return nil, errors.New("daily event publisher: nil bus")
	}
	return &DailyEventPublisher{bus: bus}, nil
}

// PublishStatisticCalculated publishes day statistic events from rollups.
func (p *DailyEventPublisher) PublishStatisticCalculated(ctx context.Context, event events.StatisticCalculated) error {
	return p.bus.PublishDayStatisticCalculated(ctx, event)
}

