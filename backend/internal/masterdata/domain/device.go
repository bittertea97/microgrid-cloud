package masterdata

import (
	"context"
	"errors"
	"time"
)

// Device represents a device bound to a station.
type Device struct {
	ID         string
	StationID  string
	TBEntityID string
	DeviceType string
	Name       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Validate checks device invariants.
func (d Device) Validate() error {
	if d.ID == "" {
		return errors.New("device: empty id")
	}
	if d.StationID == "" {
		return errors.New("device: empty station id")
	}
	return nil
}

// DeviceRepository manages device persistence.
type DeviceRepository interface {
	Get(ctx context.Context, id string) (*Device, error)
	ListByStation(ctx context.Context, stationID string) ([]Device, error)
	Save(ctx context.Context, device *Device) error
}
