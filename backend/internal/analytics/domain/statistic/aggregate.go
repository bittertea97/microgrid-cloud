package statistic

import "time"

// Granularity is the time resolution of a statistic aggregate.
type Granularity string

const (
	GranularityHour  Granularity = "HOUR"
	GranularityDay   Granularity = "DAY"
	GranularityMonth Granularity = "MONTH"
	GranularityYear  Granularity = "YEAR"
)

// StatisticID is the identity of a statistic aggregate.
type StatisticID string

// StatisticFact is the immutable statistical result once completed.
type StatisticFact struct {
	ChargeKWh       float64
	DischargeKWh    float64
	Earnings        float64
	CarbonReduction float64
}

// Validate ensures basic domain invariants for a fact.
func (f StatisticFact) Validate() error {
	if f.ChargeKWh < 0 || f.DischargeKWh < 0 || f.Earnings < 0 || f.CarbonReduction < 0 {
		return ErrNegativeFactValue
	}
	return nil
}

// StatisticAggregate is the root of the statistic domain.
// Invariants:
// 1) Only HOUR/DAY/MONTH/YEAR granularity is allowed.
// 2) Once completed, it is frozen and cannot be modified.
// 3) Completing twice is an error (idempotency guard).
// Note: The persistence unique key is subjectId + timeType + timeKey.
type StatisticAggregate struct {
	id          StatisticID
	granularity Granularity
	periodStart time.Time

	fact        StatisticFact
	completed   bool
	completedAt time.Time
}

// NewStatisticAggregate creates a new aggregate in "not completed" state.
func NewStatisticAggregate(id StatisticID, granularity Granularity, periodStart time.Time) (*StatisticAggregate, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	if !granularity.IsValid() {
		return nil, ErrInvalidGranularity
	}
	if periodStart.IsZero() {
		return nil, ErrInvalidPeriodStart
	}

	return &StatisticAggregate{
		id:          id,
		granularity: granularity,
		periodStart: periodStart,
	}, nil
}

// Complete freezes the aggregate with a fact.
func (a *StatisticAggregate) Complete(fact StatisticFact, completedAt time.Time) error {
	if a.completed {
		return ErrAlreadyCompleted
	}
	if completedAt.IsZero() {
		return ErrInvalidCompletedAt
	}
	if err := fact.Validate(); err != nil {
		return err
	}

	a.fact = fact
	a.completed = true
	a.completedAt = completedAt
	return nil
}

// ID returns aggregate identity.
func (a *StatisticAggregate) ID() StatisticID { return a.id }

// Granularity returns aggregate granularity.
func (a *StatisticAggregate) Granularity() Granularity { return a.granularity }

// TimeType is an alias for granularity, used by the unique key (subjectId + timeType + timeKey).
func (a *StatisticAggregate) TimeType() TimeType { return TimeType(a.granularity) }

// PeriodStart returns the start time of the aggregate period.
func (a *StatisticAggregate) PeriodStart() time.Time { return a.periodStart }

// TimeKey returns the storage-friendly time key for the aggregate period.
func (a *StatisticAggregate) TimeKey() (TimeKey, error) {
	return NewTimeKey(TimeType(a.granularity), a.periodStart)
}

// Fact returns the computed fact and whether it is available.
func (a *StatisticAggregate) Fact() (StatisticFact, bool) { return a.fact, a.completed }

// CompletedAt returns completion timestamp and whether it is available.
func (a *StatisticAggregate) CompletedAt() (time.Time, bool) {
	if !a.completed {
		return time.Time{}, false
	}
	return a.completedAt, true
}

// IsCompleted tells if the aggregate is frozen.
func (a *StatisticAggregate) IsCompleted() bool { return a.completed }

// IsValid checks if the granularity is one of the supported values.
func (g Granularity) IsValid() bool {
	switch g {
	case GranularityHour, GranularityDay, GranularityMonth, GranularityYear:
		return true
	default:
		return false
	}
}
