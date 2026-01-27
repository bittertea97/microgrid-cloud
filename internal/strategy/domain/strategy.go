package strategy

import "time"

const (
	ModeAuto   = "auto"
	ModeManual = "manual"
)

// Strategy represents a station strategy config.
type Strategy struct {
	StationID  string
	Mode       string
	Enabled    bool
	TemplateID string
	Version    int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
