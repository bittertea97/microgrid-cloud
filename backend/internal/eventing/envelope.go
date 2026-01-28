package eventing

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"
)

// Envelope wraps event payload with metadata.
type Envelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CorrelationID string          `json:"correlation_id"`
	TenantID      string          `json:"tenant_id"`
	StationID     string          `json:"station_id"`
	SchemaVersion int             `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}

// Meta provides envelope overrides.
type Meta struct {
	EventID       string
	OccurredAt    time.Time
	CorrelationID string
	TenantID      string
	StationID     string
	SchemaVersion int
}

// BuildEnvelope constructs an envelope from event payload and metadata.
func BuildEnvelope(event any, meta Meta) (Envelope, error) {
	if event == nil {
		return Envelope{}, errors.New("eventing: nil event")
	}

	eventType := reflect.TypeOf(event)
	for eventType.Kind() == reflect.Ptr {
		eventType = eventType.Elem()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return Envelope{}, err
	}

	stationID := meta.StationID
	if stationID == "" {
		stationID = extractStringField(event, "StationID", "SubjectID")
	}
	occurredAt := meta.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = extractTimeField(event, "OccurredAt")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	eventID := meta.EventID
	if eventID == "" {
		eventID = newEventID()
	}

	correlationID := meta.CorrelationID
	if correlationID == "" {
		correlationID = eventID
	}

	schemaVersion := meta.SchemaVersion
	if schemaVersion == 0 {
		schemaVersion = 1
	}

	return Envelope{
		EventID:       eventID,
		EventType:     eventType.String(),
		OccurredAt:    occurredAt.UTC(),
		CorrelationID: correlationID,
		TenantID:      meta.TenantID,
		StationID:     stationID,
		SchemaVersion: schemaVersion,
		Payload:       payload,
	}, nil
}

func extractStringField(event any, names ...string) string {
	value := reflect.ValueOf(event)
	for value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return ""
	}
	for _, name := range names {
		field := value.FieldByName(name)
		if field.IsValid() && field.Kind() == reflect.String {
			return field.String()
		}
	}
	return ""
}

func extractTimeField(event any, name string) time.Time {
	value := reflect.ValueOf(event)
	for value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return time.Time{}
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return time.Time{}
	}
	field := value.FieldByName(name)
	if !field.IsValid() {
		return time.Time{}
	}
	if t, ok := field.Interface().(time.Time); ok {
		return t
	}
	return time.Time{}
}
