package events

import (
	"encoding/json"
	"time"
)

// CommandIssued is emitted when a command is created.
type CommandIssued struct {
	EventID        string          `json:"event_id"`
	CommandID      string          `json:"command_id"`
	TenantID       string          `json:"tenant_id"`
	StationID      string          `json:"station_id"`
	DeviceID       string          `json:"device_id"`
	CommandType    string          `json:"command_type"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotency_key"`
	OccurredAt     time.Time       `json:"occurred_at"`
}

// CommandAcked is emitted when TB acknowledges the command.
type CommandAcked struct {
	EventID    string    `json:"event_id"`
	CommandID  string    `json:"command_id"`
	TenantID   string    `json:"tenant_id"`
	StationID  string    `json:"station_id"`
	DeviceID   string    `json:"device_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

// CommandFailed is emitted when TB fails to execute the command.
type CommandFailed struct {
	EventID    string    `json:"event_id"`
	CommandID  string    `json:"command_id"`
	TenantID   string    `json:"tenant_id"`
	StationID  string    `json:"station_id"`
	DeviceID   string    `json:"device_id"`
	Error      string    `json:"error"`
	OccurredAt time.Time `json:"occurred_at"`
}
