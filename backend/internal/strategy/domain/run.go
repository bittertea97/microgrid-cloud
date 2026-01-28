package strategy

import "time"

// Run records a strategy decision.
type Run struct {
	StrategyID string
	TS         time.Time
	Decision   []byte
	CommandID  string
	Status     string
	CreatedAt  time.Time
}
