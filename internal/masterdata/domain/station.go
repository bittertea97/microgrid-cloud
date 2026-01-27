package masterdata

import (
	"context"
	"errors"
	"time"
)

// Station represents a site in masterdata.
type Station struct {
	ID          string
	TenantID    string
	Name        string
	Timezone    string
	StationType string
	Region      string
	TBAssetID   string
	TBTenantID  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Validate checks station invariants.
func (s Station) Validate() error {
	if s.ID == "" {
		return errors.New("station: empty id")
	}
	if s.TenantID == "" {
		return errors.New("station: empty tenant id")
	}
	if s.Name == "" {
		return errors.New("station: empty name")
	}
	if s.Timezone == "" {
		return errors.New("station: empty timezone")
	}
	return nil
}

// StationRepository manages station persistence.
type StationRepository interface {
	Get(ctx context.Context, id string) (*Station, error)
	Save(ctx context.Context, station *Station) error
}
