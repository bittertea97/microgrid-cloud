package settlement

import "time"

// SettlementID is the identity of a settlement aggregate.
type SettlementID string

// SettlementAggregate is the root of the settlement domain for a day.
// Identity: subjectId + day time key.
type SettlementAggregate struct {
	id        SettlementID
	subjectID string
	dayStart  time.Time
	timeKey   TimeKey

	energyKWh float64
	amount    float64

	isNew bool
}

// BuildSettlementID builds the aggregate identity from subject and day start.
func BuildSettlementID(subjectID string, dayStart time.Time) (SettlementID, error) {
	if subjectID == "" {
		return "", ErrEmptySubjectID
	}
	key, err := NewDayTimeKey(dayStart)
	if err != nil {
		return "", err
	}
	return SettlementID(subjectID + "|" + key.String()), nil
}

// NewDaySettlementAggregate creates a new settlement aggregate for a day.
func NewDaySettlementAggregate(subjectID string, dayStart time.Time) (*SettlementAggregate, error) {
	id, err := BuildSettlementID(subjectID, dayStart)
	if err != nil {
		return nil, err
	}
	key, err := NewDayTimeKey(dayStart)
	if err != nil {
		return nil, err
	}

	return &SettlementAggregate{
		id:        id,
		subjectID: subjectID,
		dayStart:  dayStart,
		timeKey:   key,
		isNew:     true,
	}, nil
}

// Recalculate overwrites the settlement values.
func (a *SettlementAggregate) Recalculate(energyKWh, amount float64) error {
	if energyKWh < 0 || amount < 0 {
		return ErrNegativeValue
	}
	a.energyKWh = energyKWh
	a.amount = amount
	return nil
}

// ID returns aggregate identity.
func (a *SettlementAggregate) ID() SettlementID { return a.id }

// SubjectID returns the subject id.
func (a *SettlementAggregate) SubjectID() string { return a.subjectID }

// DayStart returns the day start.
func (a *SettlementAggregate) DayStart() time.Time { return a.dayStart }

// TimeKey returns the day time key.
func (a *SettlementAggregate) TimeKey() string { return a.timeKey.String() }

// EnergyKWh returns the day energy.
func (a *SettlementAggregate) EnergyKWh() float64 { return a.energyKWh }

// Amount returns the settlement amount.
func (a *SettlementAggregate) Amount() float64 { return a.amount }

// IsNew reports whether the aggregate was freshly created.
func (a *SettlementAggregate) IsNew() bool { return a.isNew }

// MarkPersisted marks the aggregate as persisted.
func (a *SettlementAggregate) MarkPersisted() {
	if a != nil {
		a.isNew = false
	}
}

// Clone returns a detached copy marked as persisted.
func (a *SettlementAggregate) Clone() *SettlementAggregate {
	if a == nil {
		return nil
	}
	copy := *a
	copy.isNew = false
	return &copy
}
