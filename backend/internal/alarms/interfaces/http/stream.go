package http

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	alarmapp "microgrid-cloud/internal/alarms/application"
)

// SSEBroker fans out alarm events to connected clients.
type SSEBroker struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

// NewSSEBroker constructs a broker.
func NewSSEBroker() *SSEBroker {
	return &SSEBroker{clients: make(map[chan []byte]struct{})}
}

// Notify implements AlarmNotifier.
func (b *SSEBroker) Notify(_ context.Context, event alarmapp.AlarmEvent) {
	if b == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	b.broadcast(payload)
}

// Subscribe registers a new client channel.
func (b *SSEBroker) Subscribe() chan []byte {
	if b == nil {
		return nil
	}
	ch := make(chan []byte, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a client channel.
func (b *SSEBroker) Unsubscribe(ch chan []byte) {
	if b == nil || ch == nil {
		return
	}
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *SSEBroker) broadcast(payload []byte) {
	b.mu.Lock()
	clients := make([]chan []byte, 0, len(b.clients))
	for ch := range b.clients {
		clients = append(clients, ch)
	}
	b.mu.Unlock()
	for _, ch := range clients {
		select {
		case ch <- payload:
		default:
		}
	}
}

// StreamHandler serves SSE alarm stream.
type StreamHandler struct {
	broker *SSEBroker
}

// NewStreamHandler constructs a stream handler.
func NewStreamHandler(broker *SSEBroker) *StreamHandler {
	return &StreamHandler{broker: broker}
}

// ServeHTTP handles GET /api/v1/alarms/stream.
func (h *StreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h == nil || h.broker == nil {
		http.Error(w, "stream not ready", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := h.broker.Subscribe()
	if ch == nil {
		http.Error(w, "stream not ready", http.StatusServiceUnavailable)
		return
	}
	defer h.broker.Unsubscribe(ch)

	_, _ = w.Write([]byte("event: ready\ndata: {}\n\n"))
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case payload, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write([]byte("event: alarm\n"))
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(payload)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-notify:
			return
		}
	}
}
