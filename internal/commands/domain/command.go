package commands

import "time"

const (
	StatusCreated = "created"
	StatusSent    = "sent"
	StatusAcked   = "acked"
	StatusFailed  = "failed"
	StatusTimeout = "timeout"
)

// Command represents a device command.
type Command struct {
	CommandID      string
	TenantID       string
	StationID      string
	DeviceID       string
	CommandType    string
	Payload        []byte
	IdempotencyKey string
	Status         string
	CreatedAt      time.Time
	SentAt         time.Time
	AckedAt        time.Time
	Error          string
}
