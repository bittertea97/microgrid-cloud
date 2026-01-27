package application

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"microgrid-cloud/internal/auth"
	commandsevents "microgrid-cloud/internal/commands/application/events"
	commands "microgrid-cloud/internal/commands/domain"
	commandsrepo "microgrid-cloud/internal/commands/infrastructure/postgres"
	"microgrid-cloud/internal/eventing"
	"microgrid-cloud/internal/observability/metrics"
)

// IssueRequest represents a command issue request.
type IssueRequest struct {
	TenantID       string          `json:"tenant_id"`
	StationID      string          `json:"station_id"`
	DeviceID       string          `json:"device_id"`
	CommandType    string          `json:"command_type"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
}

// IssueResponse is returned after issuing a command.
type IssueResponse struct {
	CommandID      string          `json:"command_id"`
	TenantID       string          `json:"tenant_id"`
	StationID      string          `json:"station_id"`
	DeviceID       string          `json:"device_id"`
	CommandType    string          `json:"command_type"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
	Status         string          `json:"status"`
	CreatedAt      time.Time       `json:"created_at"`
}

// Service handles command issuance and queries.
type Service struct {
	repo           *commandsrepo.CommandRepository
	publisher      *eventing.Publisher
	tenantID       string
	idempotencyTTL time.Duration
}

// NewService constructs a command service.
func NewService(repo *commandsrepo.CommandRepository, publisher *eventing.Publisher, tenantID string) (*Service, error) {
	if repo == nil {
		return nil, errors.New("commands: nil repo")
	}
	if publisher == nil {
		return nil, errors.New("commands: nil publisher")
	}
	if tenantID == "" {
		return nil, errors.New("commands: empty tenant id")
	}
	return &Service{
		repo:           repo,
		publisher:      publisher,
		tenantID:       tenantID,
		idempotencyTTL: 10 * time.Minute,
	}, nil
}

// IssueCommand creates a command and publishes CommandIssued.
func (s *Service) IssueCommand(ctx context.Context, req IssueRequest) (*IssueResponse, error) {
	if err := validateIssue(req); err != nil {
		return nil, err
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = req.TenantID
	}
	if tenantID == "" {
		tenantID = s.tenantID
	}
	if tenantID != "" && req.TenantID != "" && req.TenantID != tenantID {
		return nil, auth.ErrTenantMismatch
	}
	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = buildIdempotencyKey(tenantID, req.StationID, req.DeviceID, req.CommandType, req.Payload)
	}

	now := time.Now().UTC()
	existing, err := s.repo.FindByIdempotencyKey(ctx, tenantID, idempotencyKey, now.Add(-s.idempotencyTTL))
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &IssueResponse{
			CommandID:      existing.CommandID,
			TenantID:       existing.TenantID,
			StationID:      existing.StationID,
			DeviceID:       existing.DeviceID,
			CommandType:    existing.CommandType,
			Payload:        existing.Payload,
			IdempotencyKey: existing.IdempotencyKey,
			Status:         existing.Status,
			CreatedAt:      existing.CreatedAt,
		}, nil
	}

	commandID := "cmd-" + buildShortID(tenantID+req.DeviceID+req.CommandType+now.Format(time.RFC3339Nano))
	cmd := &commands.Command{
		CommandID:      commandID,
		TenantID:       tenantID,
		StationID:      req.StationID,
		DeviceID:       req.DeviceID,
		CommandType:    req.CommandType,
		Payload:        req.Payload,
		IdempotencyKey: idempotencyKey,
		Status:         commands.StatusCreated,
		CreatedAt:      now,
	}
	if err := s.repo.Create(ctx, cmd); err != nil {
		return nil, err
	}
	metrics.IncCommandIssued()

	eventID := eventing.NewEventID()
	event := commandsevents.CommandIssued{
		EventID:        eventID,
		CommandID:      commandID,
		TenantID:       tenantID,
		StationID:      req.StationID,
		DeviceID:       req.DeviceID,
		CommandType:    req.CommandType,
		Payload:        req.Payload,
		IdempotencyKey: idempotencyKey,
		OccurredAt:     now,
	}

	ctx = eventing.WithEventID(ctx, eventID)
	ctx = eventing.WithTenantID(ctx, tenantID)
	if err := s.publisher.Publish(ctx, event); err != nil {
		return nil, err
	}

	return &IssueResponse{
		CommandID:      commandID,
		TenantID:       tenantID,
		StationID:      req.StationID,
		DeviceID:       req.DeviceID,
		CommandType:    req.CommandType,
		Payload:        req.Payload,
		IdempotencyKey: idempotencyKey,
		Status:         commands.StatusCreated,
		CreatedAt:      now,
	}, nil
}

// ListCommands returns commands for a station.
func (s *Service) ListCommands(ctx context.Context, stationID string, from, to time.Time) ([]commands.Command, error) {
	if stationID == "" {
		return nil, errors.New("commands: station id required")
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = s.tenantID
	}
	return s.repo.ListByStationAndTime(ctx, tenantID, stationID, from.UTC(), to.UTC())
}

// MarkTimeouts marks commands that timed out.
func (s *Service) MarkTimeouts(ctx context.Context, before time.Time) (int, error) {
	count, err := s.repo.MarkTimeoutBefore(ctx, before)
	if err != nil {
		return count, err
	}
	metrics.AddCommandTimeouts(count)
	return count, nil
}

func validateIssue(req IssueRequest) error {
	if req.StationID == "" {
		return errors.New("commands: station_id required")
	}
	if req.DeviceID == "" {
		return errors.New("commands: device_id required")
	}
	if req.CommandType == "" {
		return errors.New("commands: command_type required")
	}
	if len(req.Payload) > 0 && !json.Valid(req.Payload) {
		return errors.New("commands: invalid payload")
	}
	return nil
}

func buildIdempotencyKey(tenantID, stationID, deviceID, commandType string, payload json.RawMessage) string {
	hash := sha1.Sum([]byte(tenantID + "|" + stationID + "|" + deviceID + "|" + commandType + "|" + string(payload)))
	return hex.EncodeToString(hash[:])
}

func buildShortID(input string) string {
	sum := sha1.Sum([]byte(input))
	return hex.EncodeToString(sum[:8])
}
