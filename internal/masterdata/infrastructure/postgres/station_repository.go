package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	masterdata "microgrid-cloud/internal/masterdata/domain"
)

const defaultStationsTable = "stations"

// StationRepository is a Postgres implementation for stations.
type StationRepository struct {
	db    DBTX
	table string
}

// NewStationRepository constructs a repository.
func NewStationRepository(db DBTX, opts ...StationOption) *StationRepository {
	repo := &StationRepository{db: db, table: defaultStationsTable}
	for _, opt := range opts {
		opt(repo)
	}
	return repo
}

// StationOption configures the repository.
type StationOption func(*StationRepository)

// WithStationTable overrides the default table name.
func WithStationTable(table string) StationOption {
	return func(repo *StationRepository) {
		if table != "" {
			repo.table = table
		}
	}
}

// Get loads a station by id.
func (r *StationRepository) Get(ctx context.Context, id string) (*masterdata.Station, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("station repo: nil db")
	}
	if id == "" {
		return nil, errors.New("station repo: empty id")
	}

	query := fmt.Sprintf(`
SELECT id, tenant_id, name, timezone, station_type, region, tb_asset_id, tb_tenant_id, created_at, updated_at
FROM %s
WHERE id = $1
LIMIT 1`, r.table)

	var station masterdata.Station
	if err := r.db.QueryRowContext(ctx, query, id).Scan(
		&station.ID,
		&station.TenantID,
		&station.Name,
		&station.Timezone,
		&station.StationType,
		&station.Region,
		&station.TBAssetID,
		&station.TBTenantID,
		&station.CreatedAt,
		&station.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	station.CreatedAt = station.CreatedAt.UTC()
	station.UpdatedAt = station.UpdatedAt.UTC()
	return &station, nil
}

// Save upserts a station.
func (r *StationRepository) Save(ctx context.Context, station *masterdata.Station) error {
	if r == nil || r.db == nil {
		return errors.New("station repo: nil db")
	}
	if station == nil {
		return errors.New("station repo: nil station")
	}
	if err := station.Validate(); err != nil {
		return err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id,
	tenant_id,
	name,
	timezone,
	station_type,
	region,
	tb_asset_id,
	tb_tenant_id
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (id)
DO UPDATE SET
	tenant_id = EXCLUDED.tenant_id,
	name = EXCLUDED.name,
	timezone = EXCLUDED.timezone,
	station_type = EXCLUDED.station_type,
	region = EXCLUDED.region,
	tb_asset_id = EXCLUDED.tb_asset_id,
	tb_tenant_id = EXCLUDED.tb_tenant_id,
	updated_at = NOW()`, r.table)

	_, err := r.db.ExecContext(
		ctx,
		query,
		station.ID,
		station.TenantID,
		station.Name,
		station.Timezone,
		station.StationType,
		station.Region,
		station.TBAssetID,
		station.TBTenantID,
	)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if station.CreatedAt.IsZero() {
		station.CreatedAt = now
	}
	station.UpdatedAt = now
	return nil
}
