package statistic

import "errors"

var (
	// ErrEmptyID is returned when the aggregate id is empty.
	ErrEmptyID = errors.New("statistic: empty id")
	// ErrInvalidGranularity is returned when granularity is unsupported.
	ErrInvalidGranularity = errors.New("statistic: invalid granularity")
	// ErrInvalidPeriodStart is returned when the period start is zero.
	ErrInvalidPeriodStart = errors.New("statistic: invalid period start")
	// ErrInvalidCompletedAt is returned when completion time is zero.
	ErrInvalidCompletedAt = errors.New("statistic: invalid completed_at")
	// ErrAlreadyCompleted guards idempotent completion.
	ErrAlreadyCompleted = errors.New("statistic: already completed")
	// ErrNegativeFactValue is returned when a fact has negative values.
	ErrNegativeFactValue = errors.New("statistic: negative fact value")
	// ErrStatisticNotFound is returned when a statistic aggregate cannot be found.
	ErrStatisticNotFound = errors.New("statistic: not found")
	// ErrDayAlreadyCompleted is returned when a day aggregate is already completed.
	ErrDayAlreadyCompleted = errors.New("statistic: day already completed")
	// ErrIncompleteHourStatistics is returned when hour aggregates are missing for a day.
	ErrIncompleteHourStatistics = errors.New("statistic: incomplete hour statistics")
	// ErrHourStatisticsNotCompleted is returned when hour aggregates are not completed.
	ErrHourStatisticsNotCompleted = errors.New("statistic: hour statistics not completed")
)
