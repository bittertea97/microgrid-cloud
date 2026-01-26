package interfaces

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"microgrid-cloud/internal/analytics/application/eventbus"
	"microgrid-cloud/internal/analytics/application/events"
)

// WindowCloseHandler publishes TelemetryWindowClosed events.
type WindowCloseHandler struct {
	bus    eventbus.EventBus
	logger *log.Logger
}

// NewWindowCloseHandler constructs the handler.
func NewWindowCloseHandler(bus eventbus.EventBus, logger *log.Logger) (*WindowCloseHandler, error) {
	if bus == nil {
		return nil, errors.New("window close handler: nil event bus")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &WindowCloseHandler{bus: bus, logger: logger}, nil
}

// ServeHTTP publishes TelemetryWindowClosed events.
func (h *WindowCloseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Printf("window close: read body error: %v", err)
		http.Error(w, "read body error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req windowCloseRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.logger.Printf("window close: decode error: %v", err)
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	windowStart, windowEnd, err := req.resolveWindow()
	if err != nil {
		h.logger.Printf("window close: invalid payload: %v", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if err := h.bus.Publish(r.Context(), events.TelemetryWindowClosed{
		StationID:   req.StationID,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		OccurredAt:  time.Now().UTC(),
		Recalculate: req.Recalculate,
	}); err != nil {
		h.logger.Printf("window close: publish error: %v", err)
		http.Error(w, "publish error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "ok",
		"windowStart": windowStart.Format(time.RFC3339),
		"windowEnd":   windowEnd.Format(time.RFC3339),
	})
}

type windowCloseRequest struct {
	StationID   string `json:"stationId"`
	WindowStart string `json:"windowStart"`
	WindowEnd   string `json:"windowEnd"`
	Recalculate bool   `json:"recalculate"`
}

func (r windowCloseRequest) resolveWindow() (time.Time, time.Time, error) {
	if r.StationID == "" {
		return time.Time{}, time.Time{}, errors.New("missing stationId")
	}
	if r.WindowStart == "" {
		return time.Time{}, time.Time{}, errors.New("missing windowStart")
	}

	start, err := time.Parse(time.RFC3339, r.WindowStart)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if r.WindowEnd != "" {
		end, err := time.Parse(time.RFC3339, r.WindowEnd)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return start.UTC(), end.UTC(), nil
	}
	return start.UTC(), start.UTC().Add(time.Hour), nil
}
