package strategy

import "time"

// Calendar defines a daily strategy window.
type Calendar struct {
	StrategyID string
	Date       time.Time
	Enabled    bool
	StartTime  time.Time
	EndTime    time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
