package interfaces

import (
	"context"
	"errors"
	"log"

	"microgrid-cloud/internal/settlement/application"
)

// LoggingPublisher logs settlement calculated events.
type LoggingPublisher struct {
	logger *log.Logger
}

// NewLoggingPublisher constructs a logging publisher.
func NewLoggingPublisher(logger *log.Logger) *LoggingPublisher {
	if logger == nil {
		logger = log.Default()
	}
	return &LoggingPublisher{logger: logger}
}

// PublishSettlementCalculated logs the event.
func (p *LoggingPublisher) PublishSettlementCalculated(ctx context.Context, event application.SettlementCalculated) error {
	_ = ctx
	if p == nil {
		return errors.New("settlement publisher: nil publisher")
	}
	p.logger.Printf("settlement calculated: station=%s day=%s amount=%.4f", event.SubjectID, event.DayStart.Format("2006-01-02"), event.Amount)
	return nil
}
