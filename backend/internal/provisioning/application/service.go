package application

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	masterdata "microgrid-cloud/internal/masterdata/domain"
	masterdatarepo "microgrid-cloud/internal/masterdata/infrastructure/postgres"
	"microgrid-cloud/internal/tbadapter"
	"time"
)

// ProvisionRequest defines station provisioning payload.
type ProvisionRequest struct {
	Station       StationInput        `json:"station"`
	Devices       []DeviceInput       `json:"devices"`
	PointMappings []PointMappingInput `json:"point_mappings"`
}

// StationInput describes a station to provision.
type StationInput struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	Type     string `json:"type"`
	Region   string `json:"region"`
}

// DeviceInput describes a device to provision.
type DeviceInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DeviceType  string `json:"device_type"`
	TBProfile   string `json:"tb_profile"`
	Credentials string `json:"credentials"`
}

// PointMappingInput describes a point mapping to persist.
type PointMappingInput struct {
	ID       string  `json:"id"`
	DeviceID string  `json:"device_id"`
	PointKey string  `json:"point_key"`
	Semantic string  `json:"semantic"`
	Unit     string  `json:"unit"`
	Factor   float64 `json:"factor"`
}

// ProvisionResponse summarizes provisioning output.
type ProvisionResponse struct {
	StationID string                `json:"station_id"`
	TB        TBProvisioningSummary `json:"tb"`
}

// TBProvisioningSummary returns tb mapping info.
type TBProvisioningSummary struct {
	TenantID string            `json:"tenant_id"`
	AssetID  string            `json:"asset_id"`
	Devices  []TBDeviceSummary `json:"devices"`
}

// TBDeviceSummary is a device mapping summary.
type TBDeviceSummary struct {
	DeviceID    string `json:"device_id"`
	TBDeviceID  string `json:"tb_device_id"`
	Credentials string `json:"credentials"`
}

// Service provisions stations and mappings.
type Service struct {
	db *sql.DB
	tb *tbadapter.Client
}

// NewService constructs a provisioning service.
func NewService(db *sql.DB, tb *tbadapter.Client) (*Service, error) {
	if db == nil {
		return nil, errors.New("provisioning: nil db")
	}
	if tb == nil {
		return nil, errors.New("provisioning: nil tb client")
	}
	return &Service{db: db, tb: tb}, nil
}

// ProvisionStation provisions masterdata and syncs TB entities.
func (s *Service) ProvisionStation(ctx context.Context, req ProvisionRequest) (*ProvisionResponse, error) {
	if err := validateProvision(req); err != nil {
		return nil, err
	}

	stationID := req.Station.ID
	if stationID == "" {
		stationID = stableID("station", req.Station.TenantID+"|"+req.Station.Name)
	}

	for i := range req.Devices {
		if req.Devices[i].ID == "" {
			req.Devices[i].ID = stableID("device", stationID+"|"+req.Devices[i].Name)
		}
	}
	for i := range req.PointMappings {
		if req.PointMappings[i].ID == "" {
			req.PointMappings[i].ID = stableID("mapping", stationID+"|"+req.PointMappings[i].DeviceID+"|"+req.PointMappings[i].PointKey+"|"+req.PointMappings[i].Semantic)
		}
		if req.PointMappings[i].Factor == 0 {
			req.PointMappings[i].Factor = 1
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	stationRepo := masterdatarepo.NewStationRepository(tx)
	deviceRepo := masterdatarepo.NewDeviceRepository(tx)
	mappingRepo := masterdatarepo.NewPointMappingRepository(tx)

	station := &masterdata.Station{
		ID:          stationID,
		TenantID:    req.Station.TenantID,
		Name:        req.Station.Name,
		Timezone:    req.Station.Timezone,
		StationType: req.Station.Type,
		Region:      req.Station.Region,
	}

	if err := stationRepo.Save(ctx, station); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	devices := make([]*masterdata.Device, 0, len(req.Devices))
	for _, input := range req.Devices {
		device := &masterdata.Device{
			ID:         input.ID,
			StationID:  stationID,
			DeviceType: input.DeviceType,
			Name:       input.Name,
		}
		if err := deviceRepo.Save(ctx, device); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		devices = append(devices, device)
	}

	for _, mapping := range req.PointMappings {
		item := &masterdata.PointMapping{
			ID:        mapping.ID,
			StationID: stationID,
			DeviceID:  mapping.DeviceID,
			PointKey:  mapping.PointKey,
			Semantic:  mapping.Semantic,
			Unit:      mapping.Unit,
			Factor:    mapping.Factor,
		}
		if err := mappingRepo.Save(ctx, item); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	tbTenant, err := s.tb.EnsureTenant(ctx, req.Station.TenantID)
	if err != nil {
		return nil, err
	}

	asset, err := s.tb.EnsureAsset(ctx, tbTenant.ID, stationID, req.Station.Name, req.Station.Type)
	if err != nil {
		return nil, err
	}

	attrs := map[string]any{
		"station_id":   stationID,
		"tenant_id":    req.Station.TenantID,
		"businessType": req.Station.Type,
		"region":       req.Station.Region,
		"timezone":     req.Station.Timezone,
	}
	if err := s.tb.SetAttributes(ctx, "ASSET", asset.ID, attrs); err != nil {
		return nil, err
	}

	result := &ProvisionResponse{
		StationID: stationID,
		TB: TBProvisioningSummary{
			TenantID: tbTenant.ID,
			AssetID:  asset.ID,
		},
	}

	for _, input := range req.Devices {
		device, err := s.tb.EnsureDevice(ctx, tbTenant.ID, input.ID, input.Name, input.DeviceType, input.TBProfile, input.Credentials)
		if err != nil {
			return nil, err
		}
		if err := s.tb.SetAttributes(ctx, "DEVICE", device.ID, map[string]any{
			"device_id":  input.ID,
			"station_id": stationID,
			"deviceType": input.DeviceType,
		}); err != nil {
			return nil, err
		}
		if err := s.tb.CreateRelation(ctx, asset.ID, device.ID); err != nil {
			return nil, err
		}

		// persist TB mapping
		if err := updateDeviceTBMapping(ctx, s.db, input.ID, device.ID); err != nil {
			return nil, err
		}

		result.TB.Devices = append(result.TB.Devices, TBDeviceSummary{
			DeviceID:    input.ID,
			TBDeviceID:  device.ID,
			Credentials: input.Credentials,
		})
	}

	if err := updateStationTBMapping(ctx, s.db, stationID, asset.ID, tbTenant.ID); err != nil {
		return nil, err
	}

	return result, nil
}

func validateProvision(req ProvisionRequest) error {
	if req.Station.TenantID == "" {
		return errors.New("provisioning: missing station tenant_id")
	}
	if req.Station.Name == "" {
		return errors.New("provisioning: missing station name")
	}
	if req.Station.Timezone == "" {
		return errors.New("provisioning: missing station timezone")
	}
	if len(req.PointMappings) == 0 {
		return errors.New("provisioning: point_mappings required")
	}
	for _, mapping := range req.PointMappings {
		if mapping.PointKey == "" || mapping.Semantic == "" || mapping.Unit == "" {
			return errors.New("provisioning: invalid point mapping")
		}
		if mapping.Factor == 0 {
			// allow default 1 in caller
			continue
		}
	}
	return nil
}

func updateStationTBMapping(ctx context.Context, db *sql.DB, stationID, assetID, tenantID string) error {
	_, err := db.ExecContext(ctx, `
UPDATE stations
SET tb_asset_id = $1, tb_tenant_id = $2, updated_at = $3
WHERE id = $4`, assetID, tenantID, time.Now().UTC(), stationID)
	return err
}

func updateDeviceTBMapping(ctx context.Context, db *sql.DB, deviceID, entityID string) error {
	_, err := db.ExecContext(ctx, `
UPDATE devices
SET tb_entity_id = $1, updated_at = $2
WHERE id = $3`, entityID, time.Now().UTC(), deviceID)
	return err
}

func stableID(prefix, key string) string {
	sum := sha1.Sum([]byte(key))
	return prefix + "-" + hex.EncodeToString(sum[:8])
}
