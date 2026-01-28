package interfaces

import (
	"context"
	"errors"
	"log"

	"microgrid-cloud/internal/analytics/application/events"
	"microgrid-cloud/internal/analytics/domain/statistic"
	"microgrid-cloud/internal/settlement/application"
)

// DayStatisticCalculatedHandler bridges analytics day statistics to settlement.
type DayStatisticCalculatedHandler struct {
	app    *application.DaySettlementApplicationService
	logger *log.Logger
}

// NewDayStatisticCalculatedHandler constructs the handler.
func NewDayStatisticCalculatedHandler(app *application.DaySettlementApplicationService, logger *log.Logger) (*DayStatisticCalculatedHandler, error) {
	if app == nil {
		return nil, errors.New("day settlement handler: nil app service")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &DayStatisticCalculatedHandler{app: app, logger: logger}, nil
}

// HandleStatisticCalculated handles analytics StatisticCalculated events.
func (h *DayStatisticCalculatedHandler) HandleStatisticCalculated(ctx context.Context, event any) error {
	if h == nil {
		return errors.New("day settlement handler: nil handler")
	}

	var evt events.StatisticCalculated
	switch e := event.(type) {
	case events.StatisticCalculated:
		evt = e
	case *events.StatisticCalculated:
		if e == nil {
			return nil
		}
		evt = *e
	default:
		return nil
	}

	if evt.Granularity != statistic.GranularityDay {
		return nil
	}

	h.logger.Printf("settlement trigger: station=%s day=%s recalc=%v", evt.StationID, evt.PeriodStart.Format("2006-01-02"), evt.Recalculate)

	return h.app.HandleDayEnergyCalculated(ctx, application.DayEnergyCalculated{
		SubjectID:   evt.StationID,
		DayStart:    evt.PeriodStart,
		Recalculate: evt.Recalculate,
		OccurredAt:  evt.OccurredAt,
	})
}
