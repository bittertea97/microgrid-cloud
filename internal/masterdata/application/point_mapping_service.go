package application

import (
	"context"
	"errors"

	masterdata "microgrid-cloud/internal/masterdata/domain"
)

// PointMappingService provides minimal point mapping commands.
type PointMappingService struct {
	repo masterdata.PointMappingRepository
}

// NewPointMappingService constructs a mapping service.
func NewPointMappingService(repo masterdata.PointMappingRepository) (*PointMappingService, error) {
	if repo == nil {
		return nil, errors.New("point mapping service: nil repository")
	}
	return &PointMappingService{repo: repo}, nil
}

// UpsertMapping validates and saves a point mapping.
func (s *PointMappingService) UpsertMapping(ctx context.Context, mapping *masterdata.PointMapping) error {
	if mapping == nil {
		return errors.New("point mapping service: nil mapping")
	}
	if err := mapping.Validate(); err != nil {
		return err
	}
	return s.repo.Save(ctx, mapping)
}
