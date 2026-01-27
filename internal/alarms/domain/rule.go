package alarms

import (
	"errors"
	"time"
)

type Operator string

const (
	OperatorGreater        Operator = ">"
	OperatorGreaterOrEqual Operator = ">="
	OperatorLess           Operator = "<"
	OperatorLessOrEqual    Operator = "<="
)

// AlarmRule defines a threshold-based alarm rule.
type AlarmRule struct {
	ID              string
	TenantID        string
	StationID       string
	Name            string
	Semantic        string
	Operator        Operator
	Threshold       float64
	Hysteresis      float64
	DurationSeconds int
	Severity        string
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Validate checks rule invariants.
func (r AlarmRule) Validate() error {
	if r.ID == "" {
		return errors.New("alarm rule: empty id")
	}
	if r.TenantID == "" {
		return errors.New("alarm rule: empty tenant id")
	}
	if r.StationID == "" {
		return errors.New("alarm rule: empty station id")
	}
	if r.Name == "" {
		return errors.New("alarm rule: empty name")
	}
	if r.Semantic == "" {
		return errors.New("alarm rule: empty semantic")
	}
	if !r.Operator.Valid() {
		return errors.New("alarm rule: invalid operator")
	}
	return nil
}

// Valid returns true when operator is supported.
func (o Operator) Valid() bool {
	switch o {
	case OperatorGreater, OperatorGreaterOrEqual, OperatorLess, OperatorLessOrEqual:
		return true
	default:
		return false
	}
}
