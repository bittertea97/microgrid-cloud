package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	commandsapp "microgrid-cloud/internal/commands/application"
	masterdata "microgrid-cloud/internal/masterdata/domain"
	strategy "microgrid-cloud/internal/strategy/domain"
	strategyrepo "microgrid-cloud/internal/strategy/infrastructure/postgres"
	telemetryadapter "microgrid-cloud/internal/strategy/adapters/telemetry"
)

const (
	StatusIssued   = "issued"
	StatusNoAction = "no_action"
	StatusSkipped  = "skipped"
	StatusError    = "error"
)

// Engine evaluates strategies and issues commands.
type Engine struct {
	repo      *strategyrepo.Repository
	telemetry *telemetryadapter.LatestReader
	commands  *commandsapp.Service
	tenantID  string
}

// NewEngine constructs an Engine.
func NewEngine(repo *strategyrepo.Repository, telemetry *telemetryadapter.LatestReader, commands *commandsapp.Service, tenantID string) (*Engine, error) {
	if repo == nil {
		return nil, errors.New("strategy engine: nil repo")
	}
	if telemetry == nil {
		return nil, errors.New("strategy engine: nil telemetry reader")
	}
	if commands == nil {
		return nil, errors.New("strategy engine: nil command service")
	}
	if tenantID == "" {
		return nil, errors.New("strategy engine: empty tenant id")
	}
	return &Engine{
		repo:      repo,
		telemetry: telemetry,
		commands:  commands,
		tenantID:  tenantID,
	}, nil
}

// Tick evaluates all enabled auto strategies at the given time.
func (e *Engine) Tick(ctx context.Context, now time.Time) error {
	if e == nil {
		return errors.New("strategy engine: nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	strategies, err := e.repo.ListAutoEnabled(ctx)
	if err != nil {
		return err
	}
	for _, item := range strategies {
		if err := e.evaluate(ctx, item, now.UTC()); err != nil {
			_ = e.repo.InsertRun(ctx, &strategy.Run{
				StrategyID: item.StationID,
				TS:         now.UTC(),
				Status:     StatusError,
				Decision:   []byte(fmt.Sprintf(`{"error":%q}`, err.Error())),
			})
		}
	}
	return nil
}

func (e *Engine) evaluate(ctx context.Context, item strategy.Strategy, now time.Time) error {
	if item.Mode != strategy.ModeAuto || !item.Enabled {
		return nil
	}

	if !e.inCalendarWindow(ctx, item.StationID, now) {
		return nil
	}

	template, err := e.repo.GetTemplate(ctx, item.TemplateID)
	if err != nil {
		return err
	}
	if template == nil {
		return errors.New("strategy engine: template not found")
	}

	switch template.Type {
	case "anti_backflow":
		return e.runAntiBackflow(ctx, item, template, now)
	default:
		return nil
	}
}

func (e *Engine) inCalendarWindow(ctx context.Context, strategyID string, now time.Time) bool {
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cal, err := e.repo.GetCalendar(ctx, strategyID, date)
	if err != nil || cal == nil {
		return false
	}
	if !cal.Enabled {
		return false
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), cal.StartTime.Hour(), cal.StartTime.Minute(), 0, 0, time.UTC)
	end := time.Date(now.Year(), now.Month(), now.Day(), cal.EndTime.Hour(), cal.EndTime.Minute(), 0, 0, time.UTC)
	if end.Before(start) || end.Equal(start) {
		return false
	}
	return now.Equal(start) || (now.After(start) && now.Before(end))
}

type antiBackflowParams struct {
	ThresholdKW float64 `json:"threshold_kw"`
	MinKW       float64 `json:"min_kw"`
	MaxKW       float64 `json:"max_kw"`
	DeviceID    string  `json:"device_id"`
	CommandType string  `json:"command_type"`
}

func (e *Engine) runAntiBackflow(ctx context.Context, item strategy.Strategy, template *strategy.Template, now time.Time) error {
	var params antiBackflowParams
	if len(template.Params) > 0 {
		_ = json.Unmarshal(template.Params, &params)
	}
	if params.CommandType == "" {
		params.CommandType = "setPower"
	}
	if params.MaxKW == 0 {
		params.MaxKW = 10000
	}

	latest, err := e.telemetry.LatestSemantic(ctx, e.tenantID, item.StationID, masterdata.SemanticGridExportKW)
	if err != nil {
		return err
	}
	decision := map[string]any{
		"station_id":       item.StationID,
		"template_id":      template.ID,
		"grid_export_kw":   latest.Value,
		"threshold_kw":     params.ThresholdKW,
		"min_kw":           params.MinKW,
		"max_kw":           params.MaxKW,
		"telemetry_ts":     latest.Timestamp.Format(time.RFC3339),
		"semantic_points":  latest.Points,
		"mode":             item.Mode,
	}

	if latest.Value <= params.ThresholdKW {
		payload, _ := json.Marshal(decision)
		return e.repo.InsertRun(ctx, &strategy.Run{
			StrategyID: item.StationID,
			TS:         now,
			Decision:   payload,
			Status:     StatusNoAction,
		})
	}

	if params.DeviceID == "" {
		decision["error"] = "missing device_id"
		payload, _ := json.Marshal(decision)
		return e.repo.InsertRun(ctx, &strategy.Run{
			StrategyID: item.StationID,
			TS:         now,
			Decision:   payload,
			Status:     StatusError,
		})
	}

	target := clamp(latest.Value, params.MinKW, params.MaxKW)
	decision["target_kw"] = target

	cmdPayload := map[string]any{"pcs_target_power_kw": target}
	payloadBytes, _ := json.Marshal(cmdPayload)
	idemKey := buildIdemKey(item.StationID, now, target)

	resp, err := e.commands.IssueCommand(ctx, commandsapp.IssueRequest{
		TenantID:       e.tenantID,
		StationID:      item.StationID,
		DeviceID:       params.DeviceID,
		CommandType:    params.CommandType,
		Payload:        payloadBytes,
		IdempotencyKey: idemKey,
	})
	if err != nil {
		decision["error"] = err.Error()
		payload, _ := json.Marshal(decision)
		return e.repo.InsertRun(ctx, &strategy.Run{
			StrategyID: item.StationID,
			TS:         now,
			Decision:   payload,
			Status:     StatusError,
		})
	}

	decision["command_id"] = resp.CommandID
	payload, _ := json.Marshal(decision)
	return e.repo.InsertRun(ctx, &strategy.Run{
		StrategyID: item.StationID,
		TS:         now,
		Decision:   payload,
		CommandID:  resp.CommandID,
		Status:     StatusIssued,
	})
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return math.Round(value*1000) / 1000
}

func buildIdemKey(stationID string, now time.Time, target float64) string {
	return fmt.Sprintf("strategy:%s:%s:%.3f", stationID, now.UTC().Format("2006-01-02T15:04"), target)
}
