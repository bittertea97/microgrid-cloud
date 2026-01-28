package notify

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	alarmapp "microgrid-cloud/internal/alarms/application"
	alarms "microgrid-cloud/internal/alarms/domain"
	masterdata "microgrid-cloud/internal/masterdata/domain"
)

// RuleReader loads alarm rules.
type RuleReader interface {
	GetByID(ctx context.Context, tenantID, ruleID string) (*alarms.AlarmRule, error)
}

// StationReader loads station metadata.
type StationReader interface {
	Get(ctx context.Context, id string) (*masterdata.Station, error)
}

// AlarmReader loads alarm records.
type AlarmReader interface {
	GetByID(ctx context.Context, id string) (*alarms.Alarm, error)
}

// Clock provides time for scheduling.
type Clock interface {
	Now() time.Time
}

// ReportURLResolver provides a report link for an alarm when available.
type ReportURLResolver func(ctx context.Context, alarm alarms.Alarm, rule *alarms.AlarmRule, station *masterdata.Station) string

type sendRecord struct {
	at   time.Time
	hash string
}

// Notifier sends alarm notifications via a channel and handles escalation.
type Notifier struct {
	rules          RuleReader
	stations       StationReader
	alarms         AlarmReader
	channel        Channel
	template       *Template
	escalation     time.Duration
	clock          Clock
	mu             sync.Mutex
	timers         map[string]*time.Timer
	sent           map[string]sendRecord
	cooldown       time.Duration
	dedupeWindow   time.Duration
	reportURL      ReportURLResolver
	requestTimeout time.Duration
}

// Option configures the notifier.
type Option func(*Notifier)

// WithEscalation configures escalation delay.
func WithEscalation(after time.Duration) Option {
	return func(n *Notifier) {
		if after > 0 {
			n.escalation = after
		}
	}
}

// WithClock overrides the default clock.
func WithClock(clock Clock) Option {
	return func(n *Notifier) {
		if clock != nil {
			n.clock = clock
		}
	}
}

// WithRequestTimeout overrides the default timeout for escalation checks.
func WithRequestTimeout(timeout time.Duration) Option {
	return func(n *Notifier) {
		if timeout > 0 {
			n.requestTimeout = timeout
		}
	}
}

// WithCooldown sets a minimum interval between notifications for the same alarm and event.
func WithCooldown(interval time.Duration) Option {
	return func(n *Notifier) {
		if interval > 0 {
			n.cooldown = interval
		}
	}
}

// WithDedupeWindow suppresses identical notifications within the window.
func WithDedupeWindow(window time.Duration) Option {
	return func(n *Notifier) {
		if window > 0 {
			n.dedupeWindow = window
		}
	}
}

// WithReportURLResolver injects a report link resolver.
func WithReportURLResolver(resolver ReportURLResolver) Option {
	return func(n *Notifier) {
		if resolver != nil {
			n.reportURL = resolver
		}
	}
}

// NewNotifier constructs an alarm notifier.
func NewNotifier(rules RuleReader, stations StationReader, alarms AlarmReader, channel Channel, template *Template, opts ...Option) (*Notifier, error) {
	if rules == nil {
		return nil, errors.New("alarm notifier: nil rule reader")
	}
	if alarms == nil {
		return nil, errors.New("alarm notifier: nil alarm reader")
	}
	if channel == nil {
		return nil, errors.New("alarm notifier: nil channel")
	}
	if template == nil {
		defaultTemplate, err := NewTemplate("")
		if err != nil {
			return nil, err
		}
		template = defaultTemplate
	}
	n := &Notifier{
		rules:          rules,
		stations:       stations,
		alarms:         alarms,
		channel:        channel,
		template:       template,
		escalation:     0,
		clock:          systemClock{},
		timers:         make(map[string]*time.Timer),
		sent:           make(map[string]sendRecord),
		requestTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(n)
	}
	return n, nil
}

// Notify implements AlarmNotifier.
func (n *Notifier) Notify(ctx context.Context, event alarmapp.AlarmEvent) {
	if n == nil || n.channel == nil {
		return
	}
	rule, station := n.lookup(ctx, event.Alarm)
	n.dispatch(ctx, event.Type, event.Alarm, rule, station)

	switch event.Type {
	case "active":
		n.scheduleEscalation(event.Alarm, rule)
	case "cleared":
		n.cancelEscalation(event.Alarm.ID)
	}
}

// Close stops all pending escalation timers.
func (n *Notifier) Close() {
	if n == nil {
		return
	}
	n.mu.Lock()
	timers := n.timers
	n.timers = make(map[string]*time.Timer)
	n.mu.Unlock()
	for _, timer := range timers {
		if timer != nil {
			timer.Stop()
		}
	}
}

func (n *Notifier) lookup(ctx context.Context, alarm alarms.Alarm) (*alarms.AlarmRule, *masterdata.Station) {
	var rule *alarms.AlarmRule
	if n.rules != nil {
		r, err := n.rules.GetByID(ctx, alarm.TenantID, alarm.RuleID)
		if err == nil {
			rule = r
		}
	}
	var station *masterdata.Station
	if n.stations != nil {
		st, err := n.stations.Get(ctx, alarm.StationID)
		if err == nil {
			station = st
		}
	}
	return rule, station
}

func (n *Notifier) dispatch(ctx context.Context, eventType string, alarm alarms.Alarm, rule *alarms.AlarmRule, station *masterdata.Station) {
	reportURL := ""
	if n != nil && n.reportURL != nil {
		reportURL = n.reportURL(ctx, alarm, rule, station)
	}
	data := buildTemplateData(eventType, alarm, rule, station, reportURL)
	content, err := n.template.Render(data)
	if err != nil {
		return
	}
	if !n.shouldSend(alarm.ID, eventType, content) {
		return
	}
	if err := n.channel.Send(ctx, content); err != nil {
		return
	}
	n.markSent(alarm.ID, eventType, content)
}

func (n *Notifier) scheduleEscalation(alarm alarms.Alarm, rule *alarms.AlarmRule) {
	if n == nil || n.escalation <= 0 || alarm.ID == "" {
		return
	}
	if rule == nil || !severityAtLeast(rule.Severity, "high") {
		return
	}
	n.mu.Lock()
	if existing, ok := n.timers[alarm.ID]; ok {
		if existing != nil {
			existing.Stop()
		}
	}
	timer := time.AfterFunc(n.escalation, func() {
		n.runEscalation(alarm.ID)
	})
	n.timers[alarm.ID] = timer
	n.mu.Unlock()
}

func (n *Notifier) cancelEscalation(alarmID string) {
	if n == nil || alarmID == "" {
		return
	}
	n.mu.Lock()
	timer := n.timers[alarmID]
	delete(n.timers, alarmID)
	n.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
}

func (n *Notifier) runEscalation(alarmID string) {
	if n == nil || alarmID == "" {
		return
	}
	n.mu.Lock()
	delete(n.timers, alarmID)
	n.mu.Unlock()

	ctx := context.Background()
	if n.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, n.requestTimeout)
		defer cancel()
	}

	alarm, err := n.alarms.GetByID(ctx, alarmID)
	if err != nil || alarm == nil {
		return
	}
	if alarm.Status == alarms.StatusCleared {
		return
	}
	rule, station := n.lookup(ctx, *alarm)
	if rule == nil || !severityAtLeast(rule.Severity, "high") {
		return
	}
	n.dispatch(ctx, "escalated", *alarm, rule, station)
}

func buildTemplateData(eventType string, alarm alarms.Alarm, rule *alarms.AlarmRule, station *masterdata.Station, reportURL string) TemplateData {
	stationName := alarm.StationID
	if station != nil && station.Name != "" {
		stationName = station.Name
	}
	ruleName := alarm.RuleID
	severity := ""
	operator := ""
	threshold := ""
	if rule != nil {
		if rule.Name != "" {
			ruleName = rule.Name
		}
		severity = rule.Severity
		operator = string(rule.Operator)
		threshold = formatFloat(rule.Threshold)
	}
	thresholdText := threshold
	if operator != "" && threshold != "" {
		thresholdText = fmt.Sprintf("%s %s", operator, threshold)
	}
	startAt := alarm.StartAt
	if startAt.IsZero() {
		startAt = alarm.CreatedAt
	}
	statusLabel := statusLabel(alarm.Status)
	suggestion := suggestionFor(rule)

	return TemplateData{
		Station:      stationName,
		StationID:    alarm.StationID,
		Rule:         ruleName,
		RuleID:       alarm.RuleID,
		TriggerValue: formatFloat(alarm.LastValue),
		Threshold:    thresholdText,
		StartTime:    startAt.UTC().Format(time.RFC3339),
		Status:       statusLabel,
		StatusCode:   alarm.Status,
		Severity:     severity,
		Suggestion:   suggestion,
		ReportURL:    reportURL,
		Event:        eventType,
		EventLabel:   eventLabel(eventType),
	}
}

func statusLabel(status string) string {
	switch status {
	case alarms.StatusActive:
		return "active"
	case alarms.StatusAcknowledged:
		return "acknowledged"
	case alarms.StatusCleared:
		return "cleared"
	default:
		return status
	}
}

func eventLabel(event string) string {
	switch event {
	case "active":
		return "Triggered"
	case "acknowledged":
		return "Acknowledged"
	case "cleared":
		return "Cleared"
	case "escalated":
		return "Escalated"
	default:
		return event
	}
}

func suggestionFor(rule *alarms.AlarmRule) string {
	if rule == nil {
		return "Inspect the station and confirm the alarm condition."
	}
	severity := strings.TrimSpace(strings.ToLower(rule.Severity))
	switch severity {
	case "critical", "high":
		return "Investigate immediately and mitigate risk."
	case "medium":
		return "Verify the condition and take action if needed."
	default:
		return "Monitor the alarm condition."
	}
}

func severityAtLeast(value, target string) bool {
	return severityRank(value) >= severityRank(target)
}

func severityRank(value string) int {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func formatFloat(value float64) string {
	return fmt.Sprintf("%.2f", value)
}

func (n *Notifier) shouldSend(alarmID, eventType, content string) bool {
	if n == nil {
		return false
	}
	if n.cooldown <= 0 && n.dedupeWindow <= 0 {
		return true
	}
	key := notificationKey(alarmID, eventType)
	now := n.clock.Now().UTC()
	hash := hashContent(content)

	n.mu.Lock()
	record, ok := n.sent[key]
	n.mu.Unlock()
	if !ok {
		return true
	}
	if n.cooldown > 0 && now.Sub(record.at) < n.cooldown {
		return false
	}
	if n.dedupeWindow > 0 && record.hash == hash && now.Sub(record.at) < n.dedupeWindow {
		return false
	}
	return true
}

func (n *Notifier) markSent(alarmID, eventType, content string) {
	if n == nil {
		return
	}
	key := notificationKey(alarmID, eventType)
	n.mu.Lock()
	n.sent[key] = sendRecord{
		at:   n.clock.Now().UTC(),
		hash: hashContent(content),
	}
	n.mu.Unlock()
}

func notificationKey(alarmID, eventType string) string {
	return alarmID + "|" + eventType
}

func hashContent(content string) string {
	sum := sha1.Sum([]byte(content))
	return hex.EncodeToString(sum[:8])
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
