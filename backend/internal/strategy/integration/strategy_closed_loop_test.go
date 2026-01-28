package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"microgrid-cloud/internal/analytics/application/eventbus"
	commandsapp "microgrid-cloud/internal/commands/application"
	commandsevents "microgrid-cloud/internal/commands/application/events"
	commandsrepo "microgrid-cloud/internal/commands/infrastructure/postgres"
	commandsinterfaces "microgrid-cloud/internal/commands/interfaces"
	"microgrid-cloud/internal/eventing"
	eventingrepo "microgrid-cloud/internal/eventing/infrastructure/postgres"
	masterdata "microgrid-cloud/internal/masterdata/domain"
	strategytelemetry "microgrid-cloud/internal/strategy/adapters/telemetry"
	strategyapp "microgrid-cloud/internal/strategy/application"
	strategyrepo "microgrid-cloud/internal/strategy/infrastructure/postgres"
	"microgrid-cloud/internal/tbadapter"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestStrategy_AntiBackflow_IssuesCommand(t *testing.T) {
	db := openDB(t)
	defer db.Close()

	if err := applyStrategyMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	ctx := context.Background()

	tenantID := "tenant-strategy"
	stationID := "station-strategy-001"
	deviceID := "device-strategy-001"

	cleanupStrategyTables(ctx, db)

	if err := seedMapping(ctx, db, stationID, deviceID, string(masterdata.SemanticGridExportKW), "grid_export_kw"); err != nil {
		t.Fatalf("seed mapping: %v", err)
	}
	if err := seedTelemetry(ctx, db, tenantID, stationID, deviceID, "grid_export_kw", 50); err != nil {
		t.Fatalf("seed telemetry: %v", err)
	}

	fake := newFakeRPCServer()
	server := httptest.NewServer(fake)
	defer server.Close()

	tbClient, err := tbadapter.NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("tb client: %v", err)
	}

	baseBus := eventbus.NewInMemoryBus()
	registry := eventing.NewRegistry()
	registry.Register(commandsevents.CommandIssued{})
	registry.Register(commandsevents.CommandAcked{})
	registry.Register(commandsevents.CommandFailed{})

	outbox := eventingrepo.NewOutboxStore(db)
	processed := eventingrepo.NewProcessedStore(db)
	dlq := eventingrepo.NewDLQStore(db)
	dispatcher := eventing.NewDispatcher(baseBus, outbox, registry, dlq)
	publisher := eventing.NewPublisher(outbox, tenantID, baseBus)

	commandRepo := commandsrepo.NewCommandRepository(db)
	commandService, err := commandsapp.NewService(commandRepo, publisher, tenantID)
	if err != nil {
		t.Fatalf("command service: %v", err)
	}
	consumer, err := commandsinterfaces.NewTBRPCConsumer(commandRepo, tbClient, publisher, nil)
	if err != nil {
		t.Fatalf("command consumer: %v", err)
	}
	eventing.Subscribe(baseBus, eventbus.EventTypeOf[commandsevents.CommandIssued](), "tb.rpc", consumer.HandleCommandIssued, processed)

	strategyRepo := strategyrepo.NewRepository(db)
	strategyService, err := strategyapp.NewService(strategyRepo)
	if err != nil {
		t.Fatalf("strategy service: %v", err)
	}
	_, err = strategyService.SetMode(ctx, stationID, "auto")
	if err != nil {
		t.Fatalf("set mode: %v", err)
	}
	_, err = strategyService.SetEnabled(ctx, stationID, true, "anti_backflow", map[string]any{
		"threshold_kw": 10.0,
		"min_kw":       0.0,
		"max_kw":       100.0,
		"device_id":    deviceID,
		"command_type": "ack",
	})
	if err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	today := time.Now().UTC()
	if err := strategyService.SetCalendar(ctx, stationID, today, true, time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(0, 1, 1, 23, 59, 0, 0, time.UTC)); err != nil {
		t.Fatalf("set calendar: %v", err)
	}

	latestReader := strategytelemetry.NewLatestReader(db)
	engine, err := strategyapp.NewEngine(strategyRepo, latestReader, commandService, tenantID)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	if err := engine.Tick(ctx, today); err != nil {
		t.Fatalf("tick: %v", err)
	}
	_, _ = dispatcher.Dispatch(ctx, 10)

	cmd, err := commandRepo.FindByIdempotencyKey(ctx, tenantID, "strategy:"+stationID+":"+today.Format("2006-01-02T15:04")+":50.000", today.Add(-time.Minute))
	if err != nil || cmd == nil {
		t.Fatalf("command not created")
	}
	if cmd.Status != "acked" {
		t.Fatalf("expected acked, got %s", cmd.Status)
	}

	runs, err := strategyRepo.ListRuns(ctx, stationID, today.Add(-time.Hour), today.Add(time.Hour))
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) == 0 {
		t.Fatalf("expected at least one run")
	}
	if fake.callCount(deviceID) == 0 {
		t.Fatalf("expected rpc call")
	}
}

func seedMapping(ctx context.Context, db *sql.DB, stationID, deviceID, semantic, pointKey string) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO point_mappings (
	id, station_id, device_id, point_key, semantic, unit, factor
) VALUES ($1,$2,$3,$4,$5,'kW',1)
ON CONFLICT (id)
DO UPDATE SET point_key = EXCLUDED.point_key, semantic = EXCLUDED.semantic, updated_at = NOW()`,
		"map-strategy-001", stationID, deviceID, pointKey, semantic)
	return err
}

func seedTelemetry(ctx context.Context, db *sql.DB, tenantID, stationID, deviceID, pointKey string, value float64) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO telemetry_points (
	tenant_id, station_id, device_id, point_key, ts, value_numeric
) VALUES ($1,$2,$3,$4,$5,$6)`,
		tenantID, stationID, deviceID, pointKey, time.Now().UTC(), value)
	return err
}

func cleanupStrategyTables(ctx context.Context, db *sql.DB) {
	_, _ = db.ExecContext(ctx, "DELETE FROM strategy_runs")
	_, _ = db.ExecContext(ctx, "DELETE FROM strategy_calendar")
	_, _ = db.ExecContext(ctx, "DELETE FROM strategies")
	_, _ = db.ExecContext(ctx, "DELETE FROM strategy_templates")
	_, _ = db.ExecContext(ctx, "DELETE FROM commands")
	_, _ = db.ExecContext(ctx, "DELETE FROM event_outbox")
	_, _ = db.ExecContext(ctx, "DELETE FROM processed_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM dead_letter_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM telemetry_points")
	_, _ = db.ExecContext(ctx, "DELETE FROM point_mappings")
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func applyStrategyMigrations(db *sql.DB) error {
	root := projectRoot()
	files := []string{
		filepath.Join(root, "migrations", "001_init.sql"),
		filepath.Join(root, "migrations", "003_masterdata.sql"),
		filepath.Join(root, "migrations", "005_eventing.sql"),
		filepath.Join(root, "migrations", "007_commands.sql"),
		filepath.Join(root, "migrations", "013_strategy.sql"),
	}
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(content)); err != nil {
			return err
		}
	}
	return nil
}

func projectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return filepath.Clean(filepath.Join(dir, "..", "..", ".."))
}

type fakeRPCServer struct {
	mu    sync.Mutex
	calls map[string]int
}

func newFakeRPCServer() *fakeRPCServer {
	return &fakeRPCServer{calls: make(map[string]int)}
}

func (f *fakeRPCServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/rpc/") {
		http.NotFound(w, r)
		return
	}
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/rpc/")
	f.mu.Lock()
	f.calls[deviceID]++
	f.mu.Unlock()

	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	resp := map[string]any{"status": "acked"}
	body, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (f *fakeRPCServer) callCount(deviceID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[deviceID]
}
