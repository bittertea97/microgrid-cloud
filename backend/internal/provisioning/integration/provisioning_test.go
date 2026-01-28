package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	provisioning "microgrid-cloud/internal/provisioning/application"
	provisioninghttp "microgrid-cloud/internal/provisioning/interfaces/http"
	"microgrid-cloud/internal/tbadapter"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestProvisioning_IdempotentTBMapping(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := applyProvisioningMigrations(db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM point_mappings")
	_, _ = db.ExecContext(ctx, "DELETE FROM devices")
	_, _ = db.ExecContext(ctx, "DELETE FROM stations")

	fake := newFakeTBServer()
	server := httptest.NewServer(fake)
	defer server.Close()

	client, err := tbadapter.NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("tb client: %v", err)
	}
	service, err := provisioning.NewService(db, client)
	if err != nil {
		t.Fatalf("provisioning service: %v", err)
	}
	handler, err := provisioninghttp.NewStationProvisioningHandler(service, nil)
	if err != nil {
		t.Fatalf("provisioning handler: %v", err)
	}

	req := provisioning.ProvisionRequest{
		Station: provisioning.StationInput{
			TenantID: "tenant-provision",
			Name:     "station-provision-001",
			Timezone: "UTC",
			Type:     "microgrid",
			Region:   "lab",
		},
		Devices: []provisioning.DeviceInput{
			{
				Name:        "device-a",
				DeviceType:  "inverter",
				TBProfile:   "default",
				Credentials: "token-123",
			},
		},
		PointMappings: []provisioning.PointMappingInput{
			{
				PointKey: "charge_power_kw",
				Semantic: "charge_power_kw",
				Unit:     "kW",
				Factor:   1,
			},
		},
	}

	resp1 := doProvision(t, handler, req)
	resp2 := doProvision(t, handler, req)

	if resp1.StationID != resp2.StationID {
		t.Fatalf("station id mismatch: %s vs %s", resp1.StationID, resp2.StationID)
	}
	if resp1.TB.AssetID != resp2.TB.AssetID {
		t.Fatalf("asset id mismatch: %s vs %s", resp1.TB.AssetID, resp2.TB.AssetID)
	}
	if len(resp1.TB.Devices) != 1 || len(resp2.TB.Devices) != 1 {
		t.Fatalf("device count mismatch")
	}
	if resp1.TB.Devices[0].TBDeviceID != resp2.TB.Devices[0].TBDeviceID {
		t.Fatalf("device id mismatch: %s vs %s", resp1.TB.Devices[0].TBDeviceID, resp2.TB.Devices[0].TBDeviceID)
	}

	if fake.assetCount() != 1 || fake.deviceCount() != 1 {
		t.Fatalf("expected idempotent tb entities, assets=%d devices=%d", fake.assetCount(), fake.deviceCount())
	}
}

func doProvision(t *testing.T, handler http.Handler, req provisioning.ProvisionRequest) provisioning.ProvisionResponse {
	t.Helper()
	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/provisioning/stations", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp provisioning.ProvisionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func applyProvisioningMigrations(db *sql.DB) error {
	root := projectRoot()
	files := []string{
		filepath.Join(root, "migrations", "001_init.sql"),
		filepath.Join(root, "migrations", "003_masterdata.sql"),
		filepath.Join(root, "migrations", "006_provisioning.sql"),
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

type fakeTBServer struct {
	mu       sync.Mutex
	tenantID string
	tenants  map[string]string
	assets   map[string]string
	devices  map[string]string
	attrs    map[string]map[string]any
	counter  int
}

func newFakeTBServer() *fakeTBServer {
	return &fakeTBServer{
		tenants: make(map[string]string),
		assets:  make(map[string]string),
		devices: make(map[string]string),
		attrs:   make(map[string]map[string]any),
	}
}

func (f *fakeTBServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/tenant":
		name := r.URL.Query().Get("tenantTitle")
		id, ok := f.tenants[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": map[string]any{"id": id}, "title": name})
		return
	case r.Method == http.MethodPost && r.URL.Path == "/api/tenant":
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		name := payload["title"].(string)
		id := f.nextID("tenant")
		f.tenants[name] = id
		_ = json.NewEncoder(w).Encode(map[string]any{"id": map[string]any{"id": id}, "title": name})
		return
	case r.Method == http.MethodPost && r.URL.Path == "/api/entitiesQuery/find":
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		entityType := payload["entityType"].(string)
		key := payload["key"].(string)
		value := payload["value"].(string)
		id := f.findByAttr(entityType, key, value)
		resp := map[string]any{"data": []any{}}
		if id != "" {
			resp["data"] = []any{map[string]any{"entityId": map[string]any{"id": id}}}
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/api/asset":
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		name := payload["name"].(string)
		id := f.nextID("asset")
		f.assets[id] = name
		_ = json.NewEncoder(w).Encode(map[string]any{"id": map[string]any{"id": id}, "name": name})
		return
	case r.Method == http.MethodPost && r.URL.Path == "/api/device":
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		name := payload["name"].(string)
		id := f.nextID("device")
		f.devices[id] = name
		_ = json.NewEncoder(w).Encode(map[string]any{"id": map[string]any{"id": id}, "name": name})
		return
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/plugins/telemetry/"):
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 6 {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		entityType := parts[3]
		entityID := parts[4]
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		key := entityType + ":" + entityID
		f.attrs[key] = payload
		w.WriteHeader(http.StatusOK)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/api/relation":
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		return
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeTBServer) nextID(prefix string) string {
	f.counter++
	return prefix + "-" + fmt.Sprintf("%d", f.counter)
}

func (f *fakeTBServer) findByAttr(entityType, key, value string) string {
	for attrKey, attrs := range f.attrs {
		if !strings.HasPrefix(attrKey, strings.ToUpper(entityType)+":") {
			continue
		}
		if v, ok := attrs[key]; ok && v == value {
			return strings.Split(attrKey, ":")[1]
		}
	}
	return ""
}

func (f *fakeTBServer) assetCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.assets)
}

func (f *fakeTBServer) deviceCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.devices)
}
