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
	"microgrid-cloud/internal/tbadapter"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestCommands_Acked_And_Idempotent(t *testing.T) {
	db := openDB(t)
	defer db.Close()

	if err := applyCommandMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM commands")
	_, _ = db.ExecContext(ctx, "DELETE FROM event_outbox")
	_, _ = db.ExecContext(ctx, "DELETE FROM processed_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM dead_letter_events")

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
	publisher := eventing.NewPublisher(outbox, "tenant-cmd", baseBus)

	repo := commandsrepo.NewCommandRepository(db)
	service, err := commandsapp.NewService(repo, publisher, "tenant-cmd")
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	consumer, err := commandsinterfaces.NewTBRPCConsumer(repo, tbClient, publisher, nil)
	if err != nil {
		t.Fatalf("consumer: %v", err)
	}
	eventing.Subscribe(baseBus, eventbus.EventTypeOf[commandsevents.CommandIssued](), "tb.rpc", consumer.HandleCommandIssued, processed)

	req := commandsapp.IssueRequest{
		StationID:      "station-001",
		DeviceID:       "device-001",
		CommandType:    "ack",
		Payload:        json.RawMessage(`{"value":1}`),
		IdempotencyKey: "idem-1",
	}

	resp1, err := service.IssueCommand(ctx, req)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	resp2, err := service.IssueCommand(ctx, req)
	if err != nil {
		t.Fatalf("issue duplicate: %v", err)
	}
	if resp1.CommandID != resp2.CommandID {
		t.Fatalf("idempotency mismatch: %s vs %s", resp1.CommandID, resp2.CommandID)
	}

	_, _ = dispatcher.Dispatch(ctx, 10)

	cmd, err := repo.GetByID(ctx, resp1.CommandID)
	if err != nil {
		t.Fatalf("get command: %v", err)
	}
	if cmd.Status != "acked" {
		t.Fatalf("expected acked, got %s", cmd.Status)
	}
	if fake.callCount("device-001") != 1 {
		t.Fatalf("expected one rpc call, got %d", fake.callCount("device-001"))
	}
}

func TestCommands_Timeout(t *testing.T) {
	db := openDB(t)
	defer db.Close()

	if err := applyCommandMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM commands")
	_, _ = db.ExecContext(ctx, "DELETE FROM event_outbox")
	_, _ = db.ExecContext(ctx, "DELETE FROM processed_events")
	_, _ = db.ExecContext(ctx, "DELETE FROM dead_letter_events")

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
	publisher := eventing.NewPublisher(outbox, "tenant-cmd", baseBus)

	repo := commandsrepo.NewCommandRepository(db)
	service, err := commandsapp.NewService(repo, publisher, "tenant-cmd")
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	consumer, err := commandsinterfaces.NewTBRPCConsumer(repo, tbClient, publisher, nil)
	if err != nil {
		t.Fatalf("consumer: %v", err)
	}
	eventing.Subscribe(baseBus, eventbus.EventTypeOf[commandsevents.CommandIssued](), "tb.rpc", consumer.HandleCommandIssued, processed)

	req := commandsapp.IssueRequest{
		StationID:   "station-002",
		DeviceID:    "device-002",
		CommandType: "sent",
		Payload:     json.RawMessage(`{"value":2}`),
	}
	resp, err := service.IssueCommand(ctx, req)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	_, _ = dispatcher.Dispatch(ctx, 10)

	_, err = service.MarkTimeouts(ctx, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("mark timeout: %v", err)
	}
	cmd, err := repo.GetByID(ctx, resp.CommandID)
	if err != nil {
		t.Fatalf("get command: %v", err)
	}
	if cmd.Status != "timeout" {
		t.Fatalf("expected timeout, got %s", cmd.Status)
	}
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

func applyCommandMigrations(db *sql.DB) error {
	root := projectRoot()
	files := []string{
		filepath.Join(root, "migrations", "005_eventing.sql"),
		filepath.Join(root, "migrations", "007_commands.sql"),
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
	method, _ := payload["method"].(string)
	resp := map[string]any{"status": "acked"}
	if method == "sent" {
		resp["status"] = "sent"
	}
	body, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (f *fakeRPCServer) callCount(deviceID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[deviceID]
}
