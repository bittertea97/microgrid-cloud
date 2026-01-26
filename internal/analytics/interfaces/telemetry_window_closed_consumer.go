package interfaces

import (
	"context"
	"errors"

	"microgrid-cloud/internal/analytics/application"
	"microgrid-cloud/internal/analytics/application/events"
)

// TelemetryWindowClosedConsumer adapts MQ-style messages into the application service.
type TelemetryWindowClosedConsumer struct {
	app application.HourlyStatisticAppService
}

// NewTelemetryWindowClosedConsumer constructs a consumer adapter.
func NewTelemetryWindowClosedConsumer(app application.HourlyStatisticAppService) (*TelemetryWindowClosedConsumer, error) {
	if app == nil {
		return nil, errors.New("telemetry consumer: nil app service")
	}
	return &TelemetryWindowClosedConsumer{app: app}, nil
}

// Consume simulates consuming a TelemetryWindowClosed event from MQ.
func (c *TelemetryWindowClosedConsumer) Consume(ctx context.Context, event events.TelemetryWindowClosed) error {
	return c.app.HandleTelemetryWindowClosed(ctx, event)
}

