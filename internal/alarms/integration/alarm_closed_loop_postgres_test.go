package integration_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	alarmapp "microgrid-cloud/internal/alarms/application"
	alarms "microgrid-cloud/internal/alarms/domain"
	alarmrepo "microgrid-cloud/internal/alarms/infrastructure/postgres"
	alarminterfaces "microgrid-cloud/internal/alarms/interfaces"
	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/eventing"
	eventingrepo "microgrid-cloud/internal/eventing/infrastructure/postgres"
	masterdatarepo "microgrid-cloud/internal/masterdata/infrastructure/postgres"
	telemetryevents "microgrid-cloud/internal/telemetry/application/events"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAlarmClosedLoop_Postgres(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if !tableExists(db, "alarm_rules") ||
		!tableExists(db, "alarms") ||
		!tableExists(db, "alarm_rule_states") ||
		!tableExists(db, "stations") ||
		!tableExists(db, "point_mappings") ||
		!tableExists(db, "event_outbox") ||
		!tableExists(db, "processed_events") ||
		!tableExists(db, "dead_letter_events") {
		t.Skip("missing tables; run migrations")
	}

	ctx := context.Background()
	tenantID := "tenant-it-alarm"
	stationID := "station-it-alarm"
	deviceID := "device-it-alarm"

	_, _ = db.ExecContext(ctx, "DELETE FROM alarm_rule_states")
	_, _ = db.ExecContext(ctx, "DELETE FROM alarms")
	_, _ = db.ExecContext(ctx, "DELETE FROM alarm_rules")
	_, _ = db.ExecContext(ctx, "DELETE FROM event_outbox")
	_, _ = db.ExecContext(ctx, "DELETE FROM processed_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM dead_letter_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM point_mappings WHERE station_id = $1", stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM devices WHERE station_id = $1", stationID)
	_, _ = db.ExecContext(ctx, "DELETE FROM stations WHERE id = $1", stationID)

	if _, err := db.ExecContext(ctx, `
INSERT INTO stations (id, tenant_id, name)
VALUES ($1, $2, $3)`, stationID, tenantID, "Alarm Station"); err != nil {
		t.Fatalf("insert station: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO devices (id, station_id, name)
VALUES ($1, $2, $3)`, deviceID, stationID, "Alarm Device"); err != nil {
		t.Fatalf("insert device: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO point_mappings (id, station_id, device_id, point_key, semantic, unit, factor)
VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		"map-alarm-1", stationID, deviceID, "charge_power_kw", "charge_power_kw", "kW", 1.0); err != nil {
		t.Fatalf("insert mapping: %v", err)
	}

	ruleRepo := alarmrepo.NewAlarmRuleRepository(db)
	rule := &alarms.AlarmRule{
		ID:              "rule-alarm-1",
		TenantID:        tenantID,
		StationID:       stationID,
		Name:            "Charge High",
		Semantic:        "charge_power_kw",
		Operator:        alarms.OperatorGreater,
		Threshold:       100,
		Hysteresis:      5,
		DurationSeconds: 0,
		Severity:        "high",
		Enabled:         true,
	}
	if err := ruleRepo.Create(ctx, rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	baseBus := eventbus.NewInMemoryBus()
	registry := eventing.NewRegistry()
	registry.Register(telemetryevents.TelemetryReceived{})

	outboxStore := eventingrepo.NewOutboxStore(db)
	processedStore := eventingrepo.NewProcessedStore(db)
	dlqStore := eventingrepo.NewDLQStore(db)
	dispatcher := eventing.NewDispatcher(baseBus, outboxStore, registry, dlqStore)
	publisher := eventing.NewPublisher(outboxStore, dispatcher, tenantID, baseBus)

	alarmRepo := alarmrepo.NewAlarmRepository(db)
	alarmStateRepo := alarmrepo.NewAlarmRuleStateRepository(db)
	pointMappingRepo := masterdatarepo.NewPointMappingRepository(db)
	service, err := alarmapp.NewService(ruleRepo, alarmRepo, alarmStateRepo, pointMappingRepo, tenantID)
	if err != nil {
		t.Fatalf("new alarm service: %v", err)
	}
	consumer, err := alarminterfaces.NewTelemetryReceivedConsumer(service)
	if err != nil {
		t.Fatalf("new alarm consumer: %v", err)
	}
	eventing.Subscribe(baseBus, eventbus.EventTypeOf[telemetryevents.TelemetryReceived](), "alarms.telemetry", func(ctx context.Context, event any) error {
		evt, ok := event.(telemetryevents.TelemetryReceived)
		if !ok {
			return eventbus.ErrInvalidEventType
		}
		return consumer.Consume(ctx, evt)
	}, processedStore)

	start := time.Date(2026, time.January, 26, 9, 0, 0, 0, time.UTC)
	high := telemetryevents.TelemetryReceived{
		TenantID:   tenantID,
		StationID:  stationID,
		DeviceID:   deviceID,
		OccurredAt: start,
		Points: []telemetryevents.TelemetryPoint{{
			PointKey: "charge_power_kw",
			Value:    120,
			TS:       start,
		}},
	}
	ctx = eventing.WithTenantID(ctx, tenantID)
	if err := publisher.Publish(ctx, high); err != nil {
		t.Fatalf("publish high: %v", err)
	}
	_ = dispatcher.Dispatch(ctx, 10)

	open, err := alarmRepo.FindOpenByRuleOriginator(ctx, tenantID, rule.ID, alarms.OriginatorDevice, deviceID)
	if err != nil {
		t.Fatalf("find open: %v", err)
	}
	if open == nil {
		t.Fatalf("expected active alarm")
	}
	if open.Status != alarms.StatusActive {
		t.Fatalf("expected active status, got %s", open.Status)
	}

	recoverAt := start.Add(5 * time.Minute)
	recover := telemetryevents.TelemetryReceived{
		TenantID:   tenantID,
		StationID:  stationID,
		DeviceID:   deviceID,
		OccurredAt: recoverAt,
		Points: []telemetryevents.TelemetryPoint{{
			PointKey: "charge_power_kw",
			Value:    90,
			TS:       recoverAt,
		}},
	}
	if err := publisher.Publish(ctx, recover); err != nil {
		t.Fatalf("publish recover: %v", err)
	}
	_ = dispatcher.Dispatch(ctx, 10)

	alarm, err := alarmRepo.GetByID(ctx, open.ID)
	if err != nil {
		t.Fatalf("get alarm: %v", err)
	}
	if alarm == nil || alarm.Status != alarms.StatusCleared {
		status := "<nil>"
		if alarm != nil {
			status = alarm.Status
		}
		t.Fatalf("expected cleared alarm, got %s", status)
	}
}

func tableExists(db *sql.DB, table string) bool {
	var exists bool
	err := db.QueryRow(`
SELECT EXISTS (
	SELECT 1
	FROM information_schema.tables
	WHERE table_schema = 'public' AND table_name = $1
)`, table).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}
