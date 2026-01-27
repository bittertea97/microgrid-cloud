package interfaces

import (
	"context"
	"errors"

	alarmapp "microgrid-cloud/internal/alarms/application"
	telemetryevents "microgrid-cloud/internal/telemetry/application/events"
)

// TelemetryReceivedConsumer adapts telemetry events into alarm application service.
type TelemetryReceivedConsumer struct {
	app *alarmapp.Service
}

// NewTelemetryReceivedConsumer constructs a consumer.
func NewTelemetryReceivedConsumer(app *alarmapp.Service) (*TelemetryReceivedConsumer, error) {
	if app == nil {
		return nil, errors.New("alarms consumer: nil service")
	}
	return &TelemetryReceivedConsumer{app: app}, nil
}

// Consume handles a telemetry received event.
func (c *TelemetryReceivedConsumer) Consume(ctx context.Context, event telemetryevents.TelemetryReceived) error {
	return c.app.HandleTelemetryReceived(ctx, event)
}
