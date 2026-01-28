package masterdata

import (
	"context"
	"errors"
	"time"
)

// PointMapping binds a raw telemetry point to a semantic meaning.
type PointMapping struct {
	ID        string
	StationID string
	DeviceID  string
	PointKey  string
	Semantic  string
	Unit      string
	Factor    float64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate checks mapping invariants.
func (m PointMapping) Validate() error {
	if m.ID == "" {
		return errors.New("point mapping: empty id")
	}
	if m.StationID == "" {
		return errors.New("point mapping: empty station id")
	}
	if m.PointKey == "" {
		return errors.New("point mapping: empty point key")
	}
	if m.Semantic == "" {
		return errors.New("point mapping: empty semantic")
	}
	if m.Unit == "" {
		return errors.New("point mapping: empty unit")
	}
	return nil
}

// PointMappingRepository manages point mapping persistence.
type PointMappingRepository interface {
	ListByStation(ctx context.Context, stationID string) ([]PointMapping, error)
	Save(ctx context.Context, mapping *PointMapping) error
}
