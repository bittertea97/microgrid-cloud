package alarms

import "time"

const (
	StatusActive       = "active"
	StatusAcknowledged = "acknowledged"
	StatusCleared      = "cleared"
)

const (
	OriginatorStation = "station"
	OriginatorDevice  = "device"
)

// Alarm represents an alarm instance raised from a rule evaluation.
type Alarm struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	StationID      string    `json:"station_id"`
	OriginatorType string    `json:"originator_type"`
	OriginatorID   string    `json:"originator_id"`
	RuleID         string    `json:"rule_id"`
	Status         string    `json:"status"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at,omitempty"`
	LastValue      float64   `json:"last_value"`
	AckedAt        time.Time `json:"acked_at,omitempty"`
	ClearedAt      time.Time `json:"cleared_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AlarmRuleState tracks pending duration evaluation.
type AlarmRuleState struct {
	TenantID       string
	StationID      string
	RuleID         string
	OriginatorType string
	OriginatorID   string
	PendingSince   time.Time
	LastValue      float64
	UpdatedAt      time.Time
}
