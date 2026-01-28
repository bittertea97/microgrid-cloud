package application

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"time"

	alarms "microgrid-cloud/internal/alarms/domain"
	alarmrepo "microgrid-cloud/internal/alarms/infrastructure/postgres"
	"microgrid-cloud/internal/auth"
	"microgrid-cloud/internal/masterdata/domain"
	"microgrid-cloud/internal/observability/metrics"
	telemetryevents "microgrid-cloud/internal/telemetry/application/events"
)

// AlarmNotifier publishes alarm lifecycle events.
type AlarmNotifier interface {
	Notify(ctx context.Context, event AlarmEvent)
}

// AlarmEvent represents a lifecycle update.
type AlarmEvent struct {
	Type  string       `json:"type"`
	Alarm alarms.Alarm `json:"alarm"`
}

// Clock provides time.
type Clock interface {
	Now() time.Time
}

// Service handles alarm evaluation and state transitions.
type Service struct {
	rules    *alarmrepo.AlarmRuleRepository
	alarms   *alarmrepo.AlarmRepository
	states   *alarmrepo.AlarmRuleStateRepository
	mappings masterdata.PointMappingRepository
	notifier AlarmNotifier
	clock    Clock
	tenantID string
}

// ServiceOption customizes the alarm service.
type ServiceOption func(*Service)

// WithNotifier assigns a notifier.
func WithNotifier(notifier AlarmNotifier) ServiceOption {
	return func(s *Service) {
		s.notifier = notifier
	}
}

// WithClock assigns a clock.
func WithClock(clock Clock) ServiceOption {
	return func(s *Service) {
		s.clock = clock
	}
}

// NewService constructs an alarm service.
func NewService(rules *alarmrepo.AlarmRuleRepository, alarmsRepo *alarmrepo.AlarmRepository, states *alarmrepo.AlarmRuleStateRepository, mappings masterdata.PointMappingRepository, tenantID string, opts ...ServiceOption) (*Service, error) {
	if rules == nil || alarmsRepo == nil || states == nil {
		return nil, errors.New("alarms: nil repository")
	}
	if mappings == nil {
		return nil, errors.New("alarms: nil point mapping repo")
	}
	if tenantID == "" {
		return nil, errors.New("alarms: empty tenant id")
	}
	service := &Service{
		rules:    rules,
		alarms:   alarmsRepo,
		states:   states,
		mappings: mappings,
		tenantID: tenantID,
		clock:    systemClock{},
	}
	for _, opt := range opts {
		opt(service)
	}
	return service, nil
}

// HandleTelemetryReceived evaluates telemetry against alarm rules.
func (s *Service) HandleTelemetryReceived(ctx context.Context, evt telemetryevents.TelemetryReceived) error {
	if s == nil {
		return errors.New("alarms: nil service")
	}
	if evt.StationID == "" || evt.TenantID == "" {
		return errors.New("alarms: telemetry missing station/tenant")
	}
	if len(evt.Points) == 0 {
		return nil
	}

	mappings, err := s.mappings.ListByStation(ctx, evt.StationID)
	if err != nil {
		return err
	}
	if len(mappings) == 0 {
		return nil
	}

	rules, err := s.rules.ListEnabledByStation(ctx, evt.TenantID, evt.StationID)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}

	rulesBySemantic := make(map[string][]alarms.AlarmRule)
	for _, rule := range rules {
		rulesBySemantic[rule.Semantic] = append(rulesBySemantic[rule.Semantic], rule)
	}

	mappingByDevice := make(map[string]masterdata.PointMapping)
	mappingByStation := make(map[string]masterdata.PointMapping)
	for _, mapping := range mappings {
		if mapping.PointKey == "" || mapping.Semantic == "" {
			continue
		}
		if mapping.DeviceID != "" {
			key := mapping.DeviceID + "|" + mapping.PointKey
			mappingByDevice[key] = mapping
			continue
		}
		mappingByStation[mapping.PointKey] = mapping
	}

	type sample struct {
		value float64
		at    time.Time
	}
	semanticSamples := make(map[string]sample)

	for _, point := range evt.Points {
		mapping, ok := resolveMapping(mappingByDevice, mappingByStation, evt.DeviceID, point.PointKey)
		if !ok {
			continue
		}
		value := point.Value * mapping.Factor
		existing := semanticSamples[mapping.Semantic]
		at := point.TS
		if at.IsZero() {
			at = evt.OccurredAt
		}
		if existing.at.IsZero() || at.After(existing.at) {
			existing.at = at
		}
		existing.value += value
		semanticSamples[mapping.Semantic] = existing
	}

	originatorType := alarms.OriginatorDevice
	originatorID := evt.DeviceID
	if originatorID == "" {
		originatorType = alarms.OriginatorStation
		originatorID = evt.StationID
	}

	for semantic, sample := range semanticSamples {
		ruleList := rulesBySemantic[semantic]
		for _, rule := range ruleList {
			if err := s.evaluateRule(ctx, evt, rule, originatorType, originatorID, sample.value, sample.at); err != nil {
				return err
			}
		}
	}
	return nil
}

// AckAlarm acknowledges an alarm.
func (s *Service) AckAlarm(ctx context.Context, id string) (*alarms.Alarm, error) {
	if s == nil {
		return nil, errors.New("alarms: nil service")
	}
	if id == "" {
		return nil, errors.New("alarms: alarm id required")
	}
	alarm, err := s.alarms.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if alarm == nil {
		return nil, alarms.ErrNotFound
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = s.tenantID
	}
	if tenantID != "" && alarm.TenantID != tenantID {
		return nil, auth.ErrTenantMismatch
	}
	if alarm.Status == alarms.StatusCleared {
		return alarm, nil
	}
	if alarm.Status != alarms.StatusAcknowledged {
		ackedAt := s.clock.Now().UTC()
		if err := s.alarms.MarkAcknowledged(ctx, alarm.ID, ackedAt); err != nil {
			return nil, err
		}
		alarm.Status = alarms.StatusAcknowledged
		alarm.AckedAt = ackedAt
		alarm.UpdatedAt = ackedAt
		s.notify(ctx, "acknowledged", *alarm)
	}
	return alarm, nil
}

// ClearAlarm clears an alarm manually.
func (s *Service) ClearAlarm(ctx context.Context, id string) (*alarms.Alarm, error) {
	if s == nil {
		return nil, errors.New("alarms: nil service")
	}
	if id == "" {
		return nil, errors.New("alarms: alarm id required")
	}
	alarm, err := s.alarms.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if alarm == nil {
		return nil, alarms.ErrNotFound
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = s.tenantID
	}
	if tenantID != "" && alarm.TenantID != tenantID {
		return nil, auth.ErrTenantMismatch
	}
	if alarm.Status == alarms.StatusCleared {
		return alarm, nil
	}
	clearedAt := s.clock.Now().UTC()
	if err := s.alarms.MarkCleared(ctx, alarm.ID, alarm.LastValue, clearedAt); err != nil {
		return nil, err
	}
	alarm.Status = alarms.StatusCleared
	alarm.ClearedAt = clearedAt
	alarm.EndAt = clearedAt
	alarm.UpdatedAt = clearedAt
	s.notify(ctx, "cleared", *alarm)
	return alarm, nil
}

// ListAlarms returns alarms by station/time/status.
func (s *Service) ListAlarms(ctx context.Context, stationID, status string, from, to time.Time) ([]alarms.Alarm, error) {
	if s == nil {
		return nil, errors.New("alarms: nil service")
	}
	if stationID == "" {
		return nil, errors.New("alarms: station id required")
	}
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = s.tenantID
	}
	return s.alarms.ListByStationStatusAndTime(ctx, tenantID, stationID, status, from.UTC(), to.UTC())
}

func (s *Service) evaluateRule(ctx context.Context, evt telemetryevents.TelemetryReceived, rule alarms.AlarmRule, originatorType, originatorID string, value float64, at time.Time) error {
	open, err := s.alarms.FindOpenByRuleOriginator(ctx, evt.TenantID, rule.ID, originatorType, originatorID)
	if err != nil {
		return err
	}

	if open != nil {
		if shouldClear(rule, value) {
			clearedAt := at
			if clearedAt.IsZero() {
				clearedAt = s.clock.Now().UTC()
			}
			if err := s.alarms.MarkCleared(ctx, open.ID, value, clearedAt); err != nil {
				return err
			}
			open.Status = alarms.StatusCleared
			open.ClearedAt = clearedAt
			open.EndAt = clearedAt
			open.LastValue = value
			open.UpdatedAt = clearedAt
			s.notify(ctx, "cleared", *open)
			return nil
		}
		if err := s.alarms.UpdateLastValue(ctx, open.ID, value, atOrNow(at, s.clock)); err != nil {
			return err
		}
		return nil
	}

	if !shouldTrigger(rule, value) {
		_ = s.states.Clear(ctx, evt.TenantID, rule.ID, originatorType, originatorID)
		return nil
	}

	if rule.DurationSeconds > 0 {
		state, err := s.states.Get(ctx, evt.TenantID, rule.ID, originatorType, originatorID)
		if err != nil {
			return err
		}
		if state == nil {
			pending := alarms.AlarmRuleState{
				TenantID:       evt.TenantID,
				StationID:      evt.StationID,
				RuleID:         rule.ID,
				OriginatorType: originatorType,
				OriginatorID:   originatorID,
				PendingSince:   atOrNow(at, s.clock),
				LastValue:      value,
				UpdatedAt:      s.clock.Now().UTC(),
			}
			return s.states.Upsert(ctx, &pending)
		}
		duration := time.Duration(rule.DurationSeconds) * time.Second
		start := state.PendingSince
		if start.IsZero() {
			start = atOrNow(at, s.clock)
		}
		if atOrNow(at, s.clock).Sub(start) < duration {
			state.PendingSince = start
			state.LastValue = value
			state.UpdatedAt = s.clock.Now().UTC()
			return s.states.Upsert(ctx, state)
		}
		_ = s.states.Clear(ctx, evt.TenantID, rule.ID, originatorType, originatorID)
		return s.createAlarm(ctx, evt, rule, originatorType, originatorID, value, start)
	}

	return s.createAlarm(ctx, evt, rule, originatorType, originatorID, value, atOrNow(at, s.clock))
}

func (s *Service) createAlarm(ctx context.Context, evt telemetryevents.TelemetryReceived, rule alarms.AlarmRule, originatorType, originatorID string, value float64, startAt time.Time) error {
	if startAt.IsZero() {
		startAt = s.clock.Now().UTC()
	}
	alarmID := buildAlarmID(evt.TenantID, rule.ID, originatorID, startAt)
	alarm := &alarms.Alarm{
		ID:             alarmID,
		TenantID:       evt.TenantID,
		StationID:      evt.StationID,
		OriginatorType: originatorType,
		OriginatorID:   originatorID,
		RuleID:         rule.ID,
		Status:         alarms.StatusActive,
		StartAt:        startAt.UTC(),
		LastValue:      value,
		CreatedAt:      s.clock.Now().UTC(),
		UpdatedAt:      s.clock.Now().UTC(),
	}
	if err := s.alarms.Create(ctx, alarm); err != nil {
		return err
	}
	s.notify(ctx, "active", *alarm)
	return nil
}

func (s *Service) notify(ctx context.Context, eventType string, alarm alarms.Alarm) {
	if s == nil {
		return
	}
	metrics.IncAlarmEvent(eventType)
	if s.notifier == nil {
		return
	}
	s.notifier.Notify(ctx, AlarmEvent{Type: eventType, Alarm: alarm})
}

func shouldTrigger(rule alarms.AlarmRule, value float64) bool {
	switch rule.Operator {
	case alarms.OperatorGreater:
		return value > rule.Threshold
	case alarms.OperatorGreaterOrEqual:
		return value >= rule.Threshold
	case alarms.OperatorLess:
		return value < rule.Threshold
	case alarms.OperatorLessOrEqual:
		return value <= rule.Threshold
	default:
		return false
	}
}

func shouldClear(rule alarms.AlarmRule, value float64) bool {
	h := rule.Hysteresis
	if h < 0 {
		h = 0
	}
	switch rule.Operator {
	case alarms.OperatorGreater, alarms.OperatorGreaterOrEqual:
		return value <= rule.Threshold-h
	case alarms.OperatorLess, alarms.OperatorLessOrEqual:
		return value >= rule.Threshold+h
	default:
		return false
	}
}

func resolveMapping(deviceMap map[string]masterdata.PointMapping, stationMap map[string]masterdata.PointMapping, deviceID, pointKey string) (masterdata.PointMapping, bool) {
	if deviceID != "" {
		if mapping, ok := deviceMap[deviceID+"|"+pointKey]; ok {
			return mapping, true
		}
	}
	mapping, ok := stationMap[pointKey]
	return mapping, ok
}

func buildAlarmID(tenantID, ruleID, originatorID string, startAt time.Time) string {
	sum := sha1.Sum([]byte(tenantID + "|" + ruleID + "|" + originatorID + "|" + startAt.Format(time.RFC3339Nano)))
	return "alarm-" + hex.EncodeToString(sum[:8])
}

func atOrNow(value time.Time, clock Clock) time.Time {
	if value.IsZero() {
		return clock.Now().UTC()
	}
	return value.UTC()
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
