package application

import (
	"context"
	"errors"

	masterdata "microgrid-cloud/internal/masterdata/domain"
)

// StationService provides minimal station commands.
type StationService struct {
	repo masterdata.StationRepository
}

// NewStationService constructs a station service.
func NewStationService(repo masterdata.StationRepository) (*StationService, error) {
	if repo == nil {
		return nil, errors.New("station service: nil repository")
	}
	return &StationService{repo: repo}, nil
}

// UpsertStation validates and saves a station.
func (s *StationService) UpsertStation(ctx context.Context, station *masterdata.Station) error {
	if station == nil {
		return errors.New("station service: nil station")
	}
	if err := station.Validate(); err != nil {
		return err
	}
	return s.repo.Save(ctx, station)
}
