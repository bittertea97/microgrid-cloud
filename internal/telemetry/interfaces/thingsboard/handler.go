package thingsboard

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"microgrid-cloud/internal/telemetry/domain"
)

// IngestHandler handles telemetry ingestion from ThingsBoard webhook.
type IngestHandler struct {
	repo   telemetry.TelemetryRepository
	logger *log.Logger
}

// NewIngestHandler constructs an ingest handler.
func NewIngestHandler(repo telemetry.TelemetryRepository, logger *log.Logger) (*IngestHandler, error) {
	if repo == nil {
		return nil, errors.New("thingsboard ingest: nil repository")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &IngestHandler{repo: repo, logger: logger}, nil
}

// ServeHTTP ingests telemetry data.
func (h *IngestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Printf("telemetry ingest: read body error: %v", err)
		http.Error(w, "read body error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req ingestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.logger.Printf("telemetry ingest: decode error: %v", err)
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	measurements, err := req.toMeasurements()
	if err != nil {
		h.logger.Printf("telemetry ingest: invalid payload: %v", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if err := h.repo.InsertMeasurements(r.Context(), measurements); err != nil {
		h.logger.Printf("telemetry ingest: insert error: %v", err)
		http.Error(w, "insert error", http.StatusInternalServerError)
		return
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
