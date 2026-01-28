package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	strategy "microgrid-cloud/internal/strategy/domain"
	strategyrepo "microgrid-cloud/internal/strategy/infrastructure/postgres"
)

const defaultTemplateType = "anti_backflow"

// Service manages strategy configs and queries.
type Service struct {
	repo *strategyrepo.Repository
}

// NewService constructs a Service.
func NewService(repo *strategyrepo.Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("strategy service: nil repo")
	}
	return &Service{repo: repo}, nil
}

// SetMode sets strategy mode for a station.
func (s *Service) SetMode(ctx context.Context, stationID, mode string) (*strategy.Strategy, error) {
	if stationID == "" {
		return nil, errors.New("strategy service: station_id required")
	}
	if mode != strategy.ModeAuto && mode != strategy.ModeManual {
		return nil, errors.New("strategy service: invalid mode")
	}
	templateID := templateIDForType(defaultTemplateType)
	if err := s.ensureTemplate(ctx, templateID, defaultTemplateType, nil); err != nil {
		return nil, err
	}

	item := &strategy.Strategy{
		StationID:  stationID,
		Mode:       mode,
		Enabled:    false,
		TemplateID: templateID,
	}
	if err := s.repo.UpsertStrategy(ctx, item); err != nil {
		return nil, err
	}
	return s.repo.GetStrategy(ctx, stationID)
}

// SetEnabled enables/disables strategy and sets template params.
func (s *Service) SetEnabled(ctx context.Context, stationID string, enabled bool, templateType string, params map[string]any) (*strategy.Strategy, error) {
	if stationID == "" {
		return nil, errors.New("strategy service: station_id required")
	}
	if templateType == "" {
		templateType = defaultTemplateType
	}
	templateID := templateIDForType(templateType)
	if err := s.ensureTemplate(ctx, templateID, templateType, params); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetStrategy(ctx, stationID)
	if err != nil {
		return nil, err
	}
	mode := strategy.ModeManual
	if existing != nil && existing.Mode != "" {
		mode = existing.Mode
	}
	item := &strategy.Strategy{
		StationID:  stationID,
		Mode:       mode,
		Enabled:    enabled,
		TemplateID: templateID,
	}
	if err := s.repo.UpsertStrategy(ctx, item); err != nil {
		return nil, err
	}
	return s.repo.GetStrategy(ctx, stationID)
}

// SetCalendar sets a daily window for a station strategy.
func (s *Service) SetCalendar(ctx context.Context, stationID string, date time.Time, enabled bool, start, end time.Time) error {
	if stationID == "" {
		return errors.New("strategy service: station_id required")
	}
	cal := &strategy.Calendar{
		StrategyID: stationID,
		Date:       date,
		Enabled:    enabled,
		StartTime:  start,
		EndTime:    end,
	}
	return s.repo.UpsertCalendar(ctx, cal)
}

// ListRuns returns runs for a station strategy.
func (s *Service) ListRuns(ctx context.Context, stationID string, from, to time.Time) ([]strategy.Run, error) {
	if stationID == "" {
		return nil, errors.New("strategy service: station_id required")
	}
	return s.repo.ListRuns(ctx, stationID, from, to)
}

func (s *Service) ensureTemplate(ctx context.Context, templateID, templateType string, params map[string]any) error {
	payload := []byte("{}")
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		payload = b
	}
	template := &strategy.Template{
		ID:     templateID,
		Type:   templateType,
		Params: payload,
	}
	return s.repo.UpsertTemplate(ctx, template)
}

func templateIDForType(templateType string) string {
	return "tmpl-" + templateType
}
