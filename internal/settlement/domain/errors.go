package settlement

import "errors"

var (
	// ErrEmptySubjectID is returned when subject id is empty.
	ErrEmptySubjectID = errors.New("settlement: empty subject id")
	// ErrInvalidDayStart is returned when day start is zero.
	ErrInvalidDayStart = errors.New("settlement: invalid day start")
	// ErrNegativeValue is returned when a negative value is provided.
	ErrNegativeValue = errors.New("settlement: negative value")
	// ErrNilAggregate is returned when saving a nil aggregate.
	ErrNilAggregate = errors.New("settlement: nil aggregate")
	// ErrSettlementNotFound is returned when a settlement is not found.
	ErrSettlementNotFound = errors.New("settlement: not found")
)
