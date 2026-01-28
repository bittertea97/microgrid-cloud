package events

import (
	"time"

	"microgrid-cloud/internal/analytics/domain/statistic"
)

// TelemetryWindowClosed is raised when a telemetry hour window is closed.
// Time window uses [start, end) semantics.
type TelemetryWindowClosed struct {
	StationID   string
	WindowStart time.Time
	WindowEnd   time.Time
	OccurredAt  time.Time
	Recalculate bool
}

// StatisticCalculated is emitted when a statistic aggregate is completed and frozen.
type StatisticCalculated struct {
	StationID   string
	StatisticID statistic.StatisticID
	Granularity statistic.Granularity
	PeriodStart time.Time
	OccurredAt  time.Time
	Recalculate bool
}

