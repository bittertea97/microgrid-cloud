package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	masterdata "microgrid-cloud/internal/masterdata/domain"
)

const defaultDevicesTable = "devices"

// DeviceRepository is a Postgres implementation for devices.
type DeviceRepository struct {
	db    DBTX
	table string
}

// NewDeviceRepository constructs a repository.
func NewDeviceRepository(db DBTX, opts ...DeviceOption) *DeviceRepository {
	repo := &DeviceRepository{db: db, table: defaultDevicesTable}
	for _, opt := range opts {
		opt(repo)
	}
	return repo
}

// DeviceOption configures the repository.
type DeviceOption func(*DeviceRepository)

// WithDeviceTable overrides the default table name.
func WithDeviceTable(table string) DeviceOption {
	return func(repo *DeviceRepository) {
		if table != "" {
			repo.table = table
		}
	}
}

// Get loads a device by id.
func (r *DeviceRepository) Get(ctx context.Context, id string) (*masterdata.Device, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("device repo: nil db")
	}
	if id == "" {
		return nil, errors.New("device repo: empty id")
	}

	query := fmt.Sprintf(`
SELECT id, station_id, tb_entity_id, device_type, name, created_at, updated_at
FROM %s
WHERE id = $1
LIMIT 1`, r.table)

	var device masterdata.Device
	if err := r.db.QueryRowContext(ctx, query, id).Scan(
		&device.ID,
		&device.StationID,
		&device.TBEntityID,
		&device.DeviceType,
		&device.Name,
		&device.CreatedAt,
		&device.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	device.CreatedAt = device.CreatedAt.UTC()
	device.UpdatedAt = device.UpdatedAt.UTC()
	return &device, nil
}

// ListByStation loads devices for a station.
func (r *DeviceRepository) ListByStation(ctx context.Context, stationID string) ([]masterdata.Device, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("device repo: nil db")
	}
	if stationID == "" {
		return nil, errors.New("device repo: empty station id")
	}

	query := fmt.Sprintf(`
SELECT id, station_id, tb_entity_id, device_type, name, created_at, updated_at
FROM %s
WHERE station_id = $1
ORDER BY id ASC`, r.table)

	rows, err := r.db.QueryContext(ctx, query, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []masterdata.Device
	for rows.Next() {
		var device masterdata.Device
		if err := rows.Scan(
			&device.ID,
			&device.StationID,
			&device.TBEntityID,
			&device.DeviceType,
			&device.Name,
			&device.CreatedAt,
			&device.UpdatedAt,
		); err != nil {
			return nil, err
		}
		device.CreatedAt = device.CreatedAt.UTC()
		device.UpdatedAt = device.UpdatedAt.UTC()
		result = append(result, device)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Save upserts a device.
func (r *DeviceRepository) Save(ctx context.Context, device *masterdata.Device) error {
	if r == nil || r.db == nil {
		return errors.New("device repo: nil db")
	}
	if device == nil {
		return errors.New("device repo: nil device")
	}
	if err := device.Validate(); err != nil {
		return err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id,
	station_id,
	tb_entity_id,
	device_type,
	name
) VALUES (
	$1, $2, $3, $4, $5
)
ON CONFLICT (id)
DO UPDATE SET
	station_id = EXCLUDED.station_id,
	tb_entity_id = EXCLUDED.tb_entity_id,
	device_type = EXCLUDED.device_type,
	name = EXCLUDED.name,
	updated_at = NOW()`, r.table)

	_, err := r.db.ExecContext(
		ctx,
		query,
		device.ID,
		device.StationID,
		device.TBEntityID,
		device.DeviceType,
		device.Name,
	)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if device.CreatedAt.IsZero() {
		device.CreatedAt = now
	}
	device.UpdatedAt = now
	return nil
}
