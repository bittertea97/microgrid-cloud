package statistic

import "time"

// TimeType is the business naming for granularity.
// It is used as part of the unique key: subjectId + timeType + timeKey.
type TimeType = Granularity

const (
	TimeTypeHour  TimeType = GranularityHour
	TimeTypeDay   TimeType = GranularityDay
	TimeTypeMonth TimeType = GranularityMonth
	TimeTypeYear  TimeType = GranularityYear
)

// TimeKey is the persisted representation of a period boundary.
type TimeKey string

// NewTimeKey builds a TimeKey for the given time type and period start.
func NewTimeKey(timeType TimeType, periodStart time.Time) (TimeKey, error) {
	if !timeType.IsValid() {
		return "", ErrInvalidGranularity
	}
	if periodStart.IsZero() {
		return "", ErrInvalidPeriodStart
	}

	layout, err := timeKeyLayout(timeType)
	if err != nil {
		return "", err
	}
	return TimeKey(periodStart.Format(layout)), nil
}

// String returns the raw string for storage.
func (k TimeKey) String() string { return string(k) }

func timeKeyLayout(timeType TimeType) (string, error) {
	switch timeType {
	case TimeTypeHour:
		return "20060102T15", nil
	case TimeTypeDay:
		return "20060102", nil
	case TimeTypeMonth:
		return "200601", nil
	case TimeTypeYear:
		return "2006", nil
	default:
		return "", ErrInvalidGranularity
	}
}
