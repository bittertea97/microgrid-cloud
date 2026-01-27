package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	alarmapp "microgrid-cloud/internal/alarms/application"
	alarms "microgrid-cloud/internal/alarms/domain"
	masterdata "microgrid-cloud/internal/masterdata/domain"
)

type stubRuleRepo struct {
	rule *alarms.AlarmRule
}

func (s stubRuleRepo) GetByID(_ context.Context, _ string, _ string) (*alarms.AlarmRule, error) {
	return s.rule, nil
}

type stubStationRepo struct {
	station *masterdata.Station
}

func (s stubStationRepo) Get(_ context.Context, _ string) (*masterdata.Station, error) {
	return s.station, nil
}

type stubAlarmRepo struct {
	alarm *alarms.Alarm
}

func (s stubAlarmRepo) GetByID(_ context.Context, _ string) (*alarms.Alarm, error) {
	return s.alarm, nil
}

func TestWebhookNotifierPayload(t *testing.T) {
	payloadCh := make(chan webhookPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload webhookPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payloadCh <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	channel, err := NewWebhookChannel(server.URL)
	if err != nil {
		t.Fatalf("new webhook channel: %v", err)
	}
	tpl, err := NewTemplate("")
	if err != nil {
		t.Fatalf("new template: %v", err)
	}

	rule := &alarms.AlarmRule{
		ID:        "rule-1",
		Name:      "Charge Power High",
		Operator:  alarms.OperatorGreater,
		Threshold: 100,
		Severity:  "high",
	}
	station := &masterdata.Station{ID: "station-1", Name: "Station A"}
	alarm := &alarms.Alarm{
		ID:        "alarm-1",
		TenantID:  "tenant-1",
		StationID: "station-1",
		RuleID:    "rule-1",
		Status:    alarms.StatusActive,
		StartAt:   time.Date(2026, 1, 26, 8, 0, 0, 0, time.UTC),
		LastValue: 123.45,
		CreatedAt: time.Date(2026, 1, 26, 8, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 26, 8, 0, 0, 0, time.UTC),
	}

	notifier, err := NewNotifier(
		stubRuleRepo{rule: rule},
		stubStationRepo{station: station},
		stubAlarmRepo{alarm: alarm},
		channel,
		tpl,
		WithEscalation(0),
		WithReportURLResolver(func(_ context.Context, _ alarms.Alarm, _ *alarms.AlarmRule, _ *masterdata.Station) string {
			return "http://example.com/report"
		}),
	)
	if err != nil {
		t.Fatalf("new notifier: %v", err)
	}

	notifier.Notify(context.Background(), alarmapp.AlarmEvent{Type: "active", Alarm: *alarm})

	select {
	case payload := <-payloadCh:
		if payload.MsgType != "text" {
			t.Fatalf("expected msgtype text, got %s", payload.MsgType)
		}
		if payload.Text.Content == "" {
			t.Fatalf("expected content in payload")
		}
		content := payload.Text.Content
		checks := []string{
			"Station: Station A",
			"Rule: Charge Power High",
			"Trigger Value: 123.45",
			"Threshold: > 100.00",
			"Start Time: 2026-01-26T08:00:00Z",
			"Current Status: active",
			"Suggestion:",
			"Report: http://example.com/report",
		}
		for _, expected := range checks {
			if !strings.Contains(content, expected) {
				t.Fatalf("expected content to include %q, got %s", expected, content)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for webhook payload")
	}
}

type recordingChannel struct {
	mu       sync.Mutex
	contents []string
}

func (r *recordingChannel) Send(_ context.Context, content string) error {
	r.mu.Lock()
	r.contents = append(r.contents, content)
	r.mu.Unlock()
	return nil
}

func (r *recordingChannel) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.contents)
}

func (r *recordingChannel) Latest() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.contents) == 0 {
		return ""
	}
	return r.contents[len(r.contents)-1]
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Add(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}

func TestNotifierCooldown(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 26, 10, 0, 0, 0, time.UTC)}
	channel := &recordingChannel{}
	tpl, err := NewTemplate("")
	if err != nil {
		t.Fatalf("new template: %v", err)
	}
	rule := &alarms.AlarmRule{ID: "rule-1", Name: "Rule", Operator: alarms.OperatorGreater, Threshold: 10, Severity: "high"}
	station := &masterdata.Station{ID: "station-1", Name: "Station A"}
	alarm := &alarms.Alarm{ID: "alarm-1", TenantID: "tenant-1", StationID: "station-1", RuleID: "rule-1", Status: alarms.StatusActive, StartAt: clock.Now(), LastValue: 12}

	notifier, err := NewNotifier(
		stubRuleRepo{rule: rule},
		stubStationRepo{station: station},
		stubAlarmRepo{alarm: alarm},
		channel,
		tpl,
		WithEscalation(0),
		WithClock(clock),
		WithCooldown(10*time.Minute),
	)
	if err != nil {
		t.Fatalf("new notifier: %v", err)
	}

	notifier.Notify(context.Background(), alarmapp.AlarmEvent{Type: "active", Alarm: *alarm})
	notifier.Notify(context.Background(), alarmapp.AlarmEvent{Type: "active", Alarm: *alarm})
	if got := channel.Count(); got != 1 {
		t.Fatalf("expected 1 notification during cooldown, got %d", got)
	}

	clock.Add(11 * time.Minute)
	notifier.Notify(context.Background(), alarmapp.AlarmEvent{Type: "active", Alarm: *alarm})
	if got := channel.Count(); got != 2 {
		t.Fatalf("expected 2 notifications after cooldown, got %d", got)
	}
}

func TestNotifierDedupeWindow(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 26, 11, 0, 0, 0, time.UTC)}
	channel := &recordingChannel{}
	tpl, err := NewTemplate("")
	if err != nil {
		t.Fatalf("new template: %v", err)
	}
	rule := &alarms.AlarmRule{ID: "rule-2", Name: "Rule", Operator: alarms.OperatorGreater, Threshold: 10, Severity: "high"}
	station := &masterdata.Station{ID: "station-1", Name: "Station A"}
	alarm := &alarms.Alarm{ID: "alarm-2", TenantID: "tenant-1", StationID: "station-1", RuleID: "rule-2", Status: alarms.StatusActive, StartAt: clock.Now(), LastValue: 12}

	notifier, err := NewNotifier(
		stubRuleRepo{rule: rule},
		stubStationRepo{station: station},
		stubAlarmRepo{alarm: alarm},
		channel,
		tpl,
		WithEscalation(0),
		WithClock(clock),
		WithDedupeWindow(30*time.Minute),
	)
	if err != nil {
		t.Fatalf("new notifier: %v", err)
	}

	notifier.Notify(context.Background(), alarmapp.AlarmEvent{Type: "active", Alarm: *alarm})
	clock.Add(5 * time.Minute)
	notifier.Notify(context.Background(), alarmapp.AlarmEvent{Type: "active", Alarm: *alarm})
	if got := channel.Count(); got != 1 {
		t.Fatalf("expected 1 notification during dedupe window, got %d", got)
	}

	alarm.LastValue = 15
	notifier.Notify(context.Background(), alarmapp.AlarmEvent{Type: "active", Alarm: *alarm})
	if got := channel.Count(); got != 2 {
		t.Fatalf("expected notification when content changes, got %d", got)
	}
}

func TestNotifierEscalation(t *testing.T) {
	channel := &recordingChannel{}
	tpl, err := NewTemplate("")
	if err != nil {
		t.Fatalf("new template: %v", err)
	}
	rule := &alarms.AlarmRule{ID: "rule-3", Name: "Rule", Operator: alarms.OperatorGreater, Threshold: 10, Severity: "high"}
	station := &masterdata.Station{ID: "station-1", Name: "Station A"}
	alarm := &alarms.Alarm{ID: "alarm-3", TenantID: "tenant-1", StationID: "station-1", RuleID: "rule-3", Status: alarms.StatusActive, StartAt: time.Date(2026, 1, 26, 12, 0, 0, 0, time.UTC), LastValue: 12}

	notifier, err := NewNotifier(
		stubRuleRepo{rule: rule},
		stubStationRepo{station: station},
		stubAlarmRepo{alarm: alarm},
		channel,
		tpl,
		WithEscalation(20*time.Millisecond),
		WithRequestTimeout(200*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("new notifier: %v", err)
	}

	notifier.Notify(context.Background(), alarmapp.AlarmEvent{Type: "active", Alarm: *alarm})

	deadline := time.After(300 * time.Millisecond)
	for {
		if channel.Count() >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected escalation notification, got %d", channel.Count())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if !strings.Contains(channel.Latest(), "Escalated") {
		t.Fatalf("expected escalated notification content, got %s", channel.Latest())
	}
}
