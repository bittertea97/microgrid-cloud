package application

import (
	"context"

	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
	statisticapp "microgrid-cloud/internal/analytics/application/statistic"
	"microgrid-cloud/internal/eventing"
)

// WireAnalyticsEventBus registers application handlers on the event bus.
// This is the minimal in-process wiring for the Analytics context.
func WireAnalyticsEventBus(bus eventbus.EventBus, hourly HourlyStatisticAppService, daily *statisticapp.DailyRollupAppService, processed eventing.ProcessedStore) {
	if bus == nil {
		return
	}

	if hourly != nil {
		eventing.Subscribe(bus, eventbus.EventTypeOf[events.TelemetryWindowClosed](), "analytics.hourly", func(ctx context.Context, event any) error {
			evt, ok := event.(events.TelemetryWindowClosed)
			if !ok {
				return eventbus.ErrInvalidEventType
			}
			return hourly.HandleTelemetryWindowClosed(ctx, evt)
		}, processed)
	}

	if daily != nil {
		eventing.Subscribe(bus, eventbus.EventTypeOf[events.StatisticCalculated](), "analytics.daily", func(ctx context.Context, event any) error {
			evt, ok := event.(events.StatisticCalculated)
			if !ok {
				return eventbus.ErrInvalidEventType
			}
			return daily.HandleStatisticCalculated(ctx, evt)
		}, processed)
	}
}
