package events

import "time"

// TelemetryPoint is a normalized telemetry sample.
type TelemetryPoint struct {
	PointKey string    `json:"point_key"`
	Value    float64   `json:"value"`
	Quality  string    `json:"quality,omitempty"`
	TS       time.Time `json:"ts"`
}

// TelemetryReceived is raised after telemetry ingestion.
type TelemetryReceived struct {
	EventID    string           `json:"event_id"`
	TenantID   string           `json:"tenant_id"`
	StationID  string           `json:"station_id"`
	DeviceID   string           `json:"device_id"`
	Points     []TelemetryPoint `json:"points"`
	OccurredAt time.Time        `json:"occurred_at"`
}
