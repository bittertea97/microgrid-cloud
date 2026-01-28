package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type fakeTBServer struct {
	start         time.Time
	latency       time.Duration
	defaultStatus string
	failRate      float64
	sentRate      float64

	mu         sync.Mutex
	byDevice   map[string]int64
	byStatus   map[string]int64
	totalCalls int64

	tenantSeq int64
	assetSeq  int64
	deviceSeq int64
	tenants   map[string]tbTenant
	assets    map[string]*tbEntity
	devices   map[string]*tbEntity
}

type tbTenant struct {
	ID   string
	Name string
}

type tbEntity struct {
	ID       string
	Name     string
	TenantID string
	Type     string
	Attrs    map[string]string
}

func main() {
	addr := getenvDefault("FAKE_TB_ADDR", ":18080")
	latencyMs := getenvIntDefault("FAKE_TB_LATENCY_MS", 0)
	defaultStatus := getenvDefault("FAKE_TB_STATUS", "")
	failRate := getenvFloatDefault("FAKE_TB_FAIL_RATE", 0)
	sentRate := getenvFloatDefault("FAKE_TB_SENT_RATE", 0)

	rand.Seed(time.Now().UnixNano())

	srv := &fakeTBServer{
		start:         time.Now().UTC(),
		latency:       time.Duration(latencyMs) * time.Millisecond,
		defaultStatus: defaultStatus,
		failRate:      failRate,
		sentRate:      sentRate,
		byDevice:      make(map[string]int64),
		byStatus:      make(map[string]int64),
		tenants:       make(map[string]tbTenant),
		assets:        make(map[string]*tbEntity),
		devices:       make(map[string]*tbEntity),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.handleHealth)
	mux.HandleFunc("/metrics", srv.handleMetrics)
	mux.HandleFunc("/api/tenant", srv.handleTenant)
	mux.HandleFunc("/api/asset", srv.handleAsset)
	mux.HandleFunc("/api/device", srv.handleDevice)
	mux.HandleFunc("/api/entitiesQuery/find", srv.handleEntitiesQuery)
	mux.HandleFunc("/api/plugins/telemetry/", srv.handleAttributes)
	mux.HandleFunc("/api/relation", srv.handleRelation)
	mux.HandleFunc("/api/rpc/", srv.handleRPC)

	log.Printf("fake TB RPC server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *fakeTBServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *fakeTBServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload := map[string]any{
		"started_at": s.start.Format(time.RFC3339),
		"total":      atomic.LoadInt64(&s.totalCalls),
		"by_device":  s.byDevice,
		"by_status":  s.byStatus,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *fakeTBServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/rpc/") {
		http.NotFound(w, r)
		return
	}

	deviceID := strings.TrimPrefix(r.URL.Path, "/api/rpc/")
	if s.latency > 0 {
		time.Sleep(s.latency)
	}

	status := s.pickStatus()
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if method, ok := payload["method"].(string); ok && s.defaultStatus == "" {
		switch method {
		case "sent":
			status = "sent"
		case "fail", "failed":
			status = "failed"
		}
	}

	s.recordCall(deviceID, status)

	resp := map[string]any{"status": status}
	if status == "failed" {
		resp["error"] = "fake rpc failed"
	}
	body, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (s *fakeTBServer) handleTenant(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		name := r.URL.Query().Get("tenantTitle")
		if name == "" {
			http.Error(w, "tenantTitle required", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		tenant, ok := s.tenants[name]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]any{
			"id":    map[string]string{"id": tenant.ID},
			"title": tenant.Name,
		})
	case http.MethodPost:
		var payload struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(payload.Title) == "" {
			http.Error(w, "title required", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		tenant, ok := s.tenants[payload.Title]
		if !ok {
			id := fmt.Sprintf("tenant-%d", atomic.AddInt64(&s.tenantSeq, 1))
			tenant = tbTenant{ID: id, Name: payload.Title}
			s.tenants[payload.Title] = tenant
		}
		s.mu.Unlock()
		writeJSON(w, map[string]any{
			"id":    map[string]string{"id": tenant.ID},
			"title": tenant.Name,
		})
	default:
		http.NotFound(w, r)
	}
}

func (s *fakeTBServer) handleAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		TenantID string `json:"tenantId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.TenantID) == "" {
		http.Error(w, "name and tenantId required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entity := range s.assets {
		if entity.Name == payload.Name && entity.TenantID == payload.TenantID {
			writeJSON(w, map[string]any{
				"id":   map[string]string{"id": entity.ID},
				"name": entity.Name,
			})
			return
		}
	}
	id := fmt.Sprintf("asset-%d", atomic.AddInt64(&s.assetSeq, 1))
	entity := &tbEntity{
		ID:       id,
		Name:     payload.Name,
		TenantID: payload.TenantID,
		Type:     payload.Type,
		Attrs:    make(map[string]string),
	}
	s.assets[id] = entity
	writeJSON(w, map[string]any{
		"id":   map[string]string{"id": entity.ID},
		"name": entity.Name,
	})
}

func (s *fakeTBServer) handleDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		TenantID string `json:"tenantId"`
		Profile  string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.TenantID) == "" {
		http.Error(w, "name and tenantId required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entity := range s.devices {
		if entity.Name == payload.Name && entity.TenantID == payload.TenantID {
			writeJSON(w, map[string]any{
				"id":   map[string]string{"id": entity.ID},
				"name": entity.Name,
			})
			return
		}
	}
	id := fmt.Sprintf("device-%d", atomic.AddInt64(&s.deviceSeq, 1))
	entity := &tbEntity{
		ID:       id,
		Name:     payload.Name,
		TenantID: payload.TenantID,
		Type:     payload.Type,
		Attrs:    make(map[string]string),
	}
	s.devices[id] = entity
	writeJSON(w, map[string]any{
		"id":   map[string]string{"id": entity.ID},
		"name": entity.Name,
	})
}

func (s *fakeTBServer) handleEntitiesQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var payload struct {
		EntityType string `json:"entityType"`
		TenantID   string `json:"tenantId"`
		Key        string `json:"key"`
		Value      any    `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	entityType := strings.ToUpper(strings.TrimSpace(payload.EntityType))
	tenantID := strings.TrimSpace(payload.TenantID)
	key := strings.TrimSpace(payload.Key)
	value := strings.TrimSpace(fmt.Sprint(payload.Value))
	if entityType == "" || tenantID == "" || key == "" || value == "" {
		http.Error(w, "invalid query", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var match *tbEntity
	switch entityType {
	case "ASSET":
		for _, entity := range s.assets {
			if entity.TenantID == tenantID && entity.Attrs[key] == value {
				match = entity
				break
			}
		}
	case "DEVICE":
		for _, entity := range s.devices {
			if entity.TenantID == tenantID && entity.Attrs[key] == value {
				match = entity
				break
			}
		}
	}
	if match == nil {
		writeJSON(w, map[string]any{"data": []any{}})
		return
	}
	writeJSON(w, map[string]any{
		"data": []any{
			map[string]any{
				"entityId": map[string]string{"id": match.ID},
			},
		},
	})
}

func (s *fakeTBServer) handleAttributes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/plugins/telemetry/")
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[2] != "attributes" {
		http.NotFound(w, r)
		return
	}
	entityType := strings.ToUpper(parts[0])
	entityID := parts[1]
	if entityType == "" || entityID == "" {
		http.NotFound(w, r)
		return
	}
	var attrs map[string]any
	if err := json.NewDecoder(r.Body).Decode(&attrs); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var entity *tbEntity
	switch entityType {
	case "ASSET":
		entity = s.assets[entityID]
	case "DEVICE":
		entity = s.devices[entityID]
	default:
		http.NotFound(w, r)
		return
	}
	if entity == nil {
		http.NotFound(w, r)
		return
	}
	if entity.Attrs == nil {
		entity.Attrs = make(map[string]string)
	}
	for key, value := range attrs {
		entity.Attrs[key] = fmt.Sprint(value)
	}
	w.WriteHeader(http.StatusOK)
}

func (s *fakeTBServer) handleRelation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *fakeTBServer) pickStatus() string {
	if s.defaultStatus != "" {
		return s.defaultStatus
	}
	if s.failRate > 0 && rand.Float64() < s.failRate {
		return "failed"
	}
	if s.sentRate > 0 && rand.Float64() < s.sentRate {
		return "sent"
	}
	return "acked"
}

func (s *fakeTBServer) recordCall(deviceID, status string) {
	atomic.AddInt64(&s.totalCalls, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if deviceID != "" {
		s.byDevice[deviceID]++
	}
	if status != "" {
		s.byStatus[status]++
	}
}

func getenvDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func getenvIntDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvFloatDefault(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
