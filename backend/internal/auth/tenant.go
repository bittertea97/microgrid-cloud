package auth

import (
	"context"
	"database/sql"
	"errors"

	masterdatarepo "microgrid-cloud/internal/masterdata/infrastructure/postgres"
)

var (
	// ErrTenantMismatch indicates resource belongs to a different tenant.
	ErrTenantMismatch = errors.New("tenant mismatch")
	// ErrNotFound indicates resource not found.
	ErrNotFound = errors.New("resource not found")
)

// StationTenantChecker validates station tenant ownership.
type StationTenantChecker interface {
	EnsureStationTenant(ctx context.Context, tenantID, stationID string) error
}

// StationChecker checks station ownership using masterdata.
type StationChecker struct {
	repo *masterdatarepo.StationRepository
}

// NewStationChecker constructs a StationChecker.
func NewStationChecker(db *sql.DB) *StationChecker {
	if db == nil {
		return nil
	}
	return &StationChecker{repo: masterdatarepo.NewStationRepository(db)}
}

// EnsureStationTenant verifies station belongs to tenant.
func (c *StationChecker) EnsureStationTenant(ctx context.Context, tenantID, stationID string) error {
	if c == nil || c.repo == nil {
		return nil
	}
	if tenantID == "" || stationID == "" {
		return nil
	}
	station, err := c.repo.Get(ctx, stationID)
	if err != nil {
		return err
	}
	if station == nil {
		return ErrNotFound
	}
	if station.TenantID != tenantID {
		return ErrTenantMismatch
	}
	return nil
}
