package telemetry

import (
	"context"
	"time"
)

// Measurement is a raw telemetry value written to storage.
type Measurement struct {
	TenantID  string
	StationID string
	DeviceID  string
	PointKey  string
	TS        time.Time

	ValueNumeric *float64
	ValueText    *string
	Quality      string
}

// TelemetryPoint groups measurements at the same timestamp.
type TelemetryPoint struct {
	At     time.Time
	Values map[string]float64
}

// TelemetryRepository persists telemetry measurements.
type TelemetryRepository interface {
	InsertMeasurements(ctx context.Context, measurements []Measurement) error
}

// TelemetryQuery loads telemetry measurements for rollups.
type TelemetryQuery interface {
	QueryHour(ctx context.Context, tenantID, stationID string, start, end time.Time) ([]TelemetryPoint, error)
}
