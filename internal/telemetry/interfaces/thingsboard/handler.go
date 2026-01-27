package thingsboard

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"microgrid-cloud/internal/eventing"
	"microgrid-cloud/internal/observability/metrics"
	telemetryevents "microgrid-cloud/internal/telemetry/application/events"
	"microgrid-cloud/internal/telemetry/domain"
)

// IngestHandler handles telemetry ingestion from ThingsBoard webhook.
type IngestHandler struct {
	repo      telemetry.TelemetryRepository
	publisher *eventing.Publisher
	logger    *log.Logger
}

// NewIngestHandler constructs an ingest handler.
func NewIngestHandler(repo telemetry.TelemetryRepository, publisher *eventing.Publisher, logger *log.Logger) (*IngestHandler, error) {
	if repo == nil {
		return nil, errors.New("thingsboard ingest: nil repository")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &IngestHandler{repo: repo, publisher: publisher, logger: logger}, nil
}

// ServeHTTP ingests telemetry data.
func (h *IngestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	result := metrics.IngestResultSuccess
	defer func() {
		metrics.ObserveIngest(result, time.Since(start))
	}()

	if r.Method != http.MethodPost {
		result = metrics.IngestResultError
		metrics.IncIngestError("method_not_allowed")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Printf("telemetry ingest: read body error: %v", err)
		result = metrics.IngestResultError
		metrics.IncIngestError("read_body")
		http.Error(w, "read body error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req ingestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.logger.Printf("telemetry ingest: decode error: %v", err)
		result = metrics.IngestResultError
		metrics.IncIngestError("invalid_json")
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	measurements, err := req.toMeasurements()
	if err != nil {
		h.logger.Printf("telemetry ingest: invalid payload: %v", err)
		result = metrics.IngestResultError
		metrics.IncIngestError("invalid_payload")
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if err := h.repo.InsertMeasurements(r.Context(), measurements); err != nil {
		h.logger.Printf("telemetry ingest: insert error: %v", err)
		result = metrics.IngestResultError
		metrics.IncIngestError("insert_error")
		http.Error(w, "insert error", http.StatusInternalServerError)
		return
	}

	if h.publisher != nil {
		points := make([]telemetryevents.TelemetryPoint, 0, len(measurements))
		var occurredAt time.Time
		for _, measurement := range measurements {
			if measurement.TS.After(occurredAt) {
				occurredAt = measurement.TS
			}
			value := 0.0
			if measurement.ValueNumeric != nil {
				value = *measurement.ValueNumeric
			}
			points = append(points, telemetryevents.TelemetryPoint{
				PointKey: measurement.PointKey,
				Value:    value,
				Quality:  measurement.Quality,
				TS:       measurement.TS,
			})
		}
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		event := telemetryevents.TelemetryReceived{
			EventID:    eventing.NewEventID(),
			TenantID:   req.TenantID,
			StationID:  req.StationID,
			DeviceID:   req.DeviceID,
			Points:     points,
			OccurredAt: occurredAt,
		}
		ctx := eventing.WithEventID(r.Context(), event.EventID)
		ctx = eventing.WithTenantID(ctx, req.TenantID)
		if err := h.publisher.Publish(ctx, event); err != nil {
			h.logger.Printf("telemetry ingest: publish error: %v", err)
		}
	}

	resp := map[string]any{"inserted": len(measurements)}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type ingestRequest struct {
	TenantID  string                 `json:"tenantId"`
	StationID string                 `json:"stationId"`
	DeviceID  string                 `json:"deviceId"`
	TS        int64                  `json:"ts"`
	Values    map[string]float64     `json:"values"`
	Quality   string                 `json:"quality"`
	Points    []ingestPoint          `json:"points"`
	Meta      map[string]interface{} `json:"meta"`
}

type ingestPoint struct {
	TS      int64              `json:"ts"`
	Values  map[string]float64 `json:"values"`
	Quality string             `json:"quality"`
}

func (r ingestRequest) toMeasurements() ([]telemetry.Measurement, error) {
	if r.TenantID == "" || r.StationID == "" || r.DeviceID == "" {
		return nil, errors.New("missing tenantId/stationId/deviceId")
	}

	points := r.Points
	if len(points) == 0 && r.TS != 0 {
		points = []ingestPoint{{TS: r.TS, Values: r.Values, Quality: r.Quality}}
	}
	if len(points) == 0 {
		return nil, errors.New("no telemetry points")
	}

	measurements := make([]telemetry.Measurement, 0, len(points))
	for _, point := range points {
		ts, err := parseTimestamp(point.TS)
		if err != nil {
			return nil, err
		}
		if len(point.Values) == 0 {
			return nil, errors.New("empty values")
		}
		for key, value := range point.Values {
			v := value
			measurements = append(measurements, telemetry.Measurement{
				TenantID:     r.TenantID,
				StationID:    r.StationID,
				DeviceID:     r.DeviceID,
				PointKey:     key,
				TS:           ts,
				ValueNumeric: &v,
				Quality:      point.Quality,
			})
		}
	}
	return measurements, nil
}

func parseTimestamp(value int64) (time.Time, error) {
	if value <= 0 {
		return time.Time{}, errors.New("invalid ts")
	}
	// Accept milliseconds or seconds.
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC(), nil
	}
	return time.Unix(value, 0).UTC(), nil
}
