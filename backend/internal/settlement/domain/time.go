package settlement

import "time"

// TimeKey is the persisted representation of a period boundary.
type TimeKey string

// NewDayTimeKey builds a TimeKey for the given day start.
func NewDayTimeKey(dayStart time.Time) (TimeKey, error) {
	if dayStart.IsZero() {
		return "", ErrInvalidDayStart
	}
	return TimeKey(dayStart.UTC().Format("20060102")), nil
}

// String returns the raw string for storage.
func (k TimeKey) String() string { return string(k) }
