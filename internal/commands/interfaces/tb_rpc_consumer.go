package interfaces

import (
	"context"
	"errors"
	"log"
	"time"

	commandsevents "microgrid-cloud/internal/commands/application/events"
	commandsrepo "microgrid-cloud/internal/commands/infrastructure/postgres"
	"microgrid-cloud/internal/eventing"
	"microgrid-cloud/internal/observability/metrics"
	"microgrid-cloud/internal/tbadapter"
)

// TBRPCConsumer sends commands to TB and updates statuses.
type TBRPCConsumer struct {
	repo      *commandsrepo.CommandRepository
	tb        *tbadapter.Client
	publisher *eventing.Publisher
	logger    *log.Logger
}

// NewTBRPCConsumer constructs a consumer.
func NewTBRPCConsumer(repo *commandsrepo.CommandRepository, tb *tbadapter.Client, publisher *eventing.Publisher, logger *log.Logger) (*TBRPCConsumer, error) {
	if repo == nil || tb == nil || publisher == nil {
		return nil, errors.New("tb rpc consumer: nil dependency")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &TBRPCConsumer{repo: repo, tb: tb, publisher: publisher, logger: logger}, nil
}

// HandleCommandIssued handles CommandIssued events.
func (c *TBRPCConsumer) HandleCommandIssued(ctx context.Context, event any) error {
	evt, ok := event.(commandsevents.CommandIssued)
	if !ok {
		if ptr, ok := event.(*commandsevents.CommandIssued); ok && ptr != nil {
			evt = *ptr
		} else {
			return nil
		}
	}

	now := time.Now().UTC()
	if err := c.repo.MarkSent(ctx, evt.CommandID, now); err != nil {
		return err
	}

	resp, err := c.tb.SendRPC(ctx, evt.DeviceID, evt.CommandType, evt.Payload)
	if err != nil {
		_ = c.repo.MarkFailed(ctx, evt.CommandID, err.Error())
		return c.publishFailed(ctx, evt, err.Error())
	}
	if resp.Status == "failed" {
		message := resp.Error
		if message == "" {
			message = "tb rpc failed"
		}
		_ = c.repo.MarkFailed(ctx, evt.CommandID, message)
		return c.publishFailed(ctx, evt, message)
	}
	if resp.Status == "acked" {
		if err := c.repo.MarkAcked(ctx, evt.CommandID, now); err != nil {
			return err
		}
		return c.publishAcked(ctx, evt)
	}

	// If TB returns "sent" or unknown status, keep as sent and rely on timeout scanner.
	c.logger.Printf("tb rpc pending: command=%s status=%s", evt.CommandID, resp.Status)
	return nil
}

func (c *TBRPCConsumer) publishAcked(ctx context.Context, evt commandsevents.CommandIssued) error {
	eventID := eventing.NewEventID()
	ack := commandsevents.CommandAcked{
		EventID:    eventID,
		CommandID:  evt.CommandID,
		TenantID:   evt.TenantID,
		StationID:  evt.StationID,
		DeviceID:   evt.DeviceID,
		OccurredAt: time.Now().UTC(),
	}
	metrics.IncCommandResult(metrics.CommandResultAcked)
	ctx = eventing.WithEventID(ctx, eventID)
	ctx = eventing.WithTenantID(ctx, evt.TenantID)
	return c.publisher.Publish(ctx, ack)
}

func (c *TBRPCConsumer) publishFailed(ctx context.Context, evt commandsevents.CommandIssued, message string) error {
	eventID := eventing.NewEventID()
	failed := commandsevents.CommandFailed{
		EventID:    eventID,
		CommandID:  evt.CommandID,
		TenantID:   evt.TenantID,
		StationID:  evt.StationID,
		DeviceID:   evt.DeviceID,
		Error:      message,
		OccurredAt: time.Now().UTC(),
	}
	metrics.IncCommandResult(metrics.CommandResultFailed)
	ctx = eventing.WithEventID(ctx, eventID)
	ctx = eventing.WithTenantID(ctx, evt.TenantID)
	return c.publisher.Publish(ctx, failed)
}
