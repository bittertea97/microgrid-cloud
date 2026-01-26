package interfaces

import (
	"context"
	"errors"

	"microgrid-cloud/internal/analytics/application/events"
	appstatistic "microgrid-cloud/internal/analytics/application/statistic"
)

// HourToDayRollupHandler adapts hour StatisticCalculated to daily rollup service.
type HourToDayRollupHandler struct {
	rollup *appstatistic.DailyRollupAppService
}

// NewHourToDayRollupHandler constructs a handler adapter.
func NewHourToDayRollupHandler(rollup *appstatistic.DailyRollupAppService) (*HourToDayRollupHandler, error) {
	if rollup == nil {
		return nil, errors.New("hour-to-day handler: nil rollup app service")
	}
	return &HourToDayRollupHandler{rollup: rollup}, nil
}

// Handle maps the hour statistic event into the daily rollup app service.
func (h *HourToDayRollupHandler) Handle(ctx context.Context, event events.StatisticCalculated) error {
	return h.rollup.HandleStatisticCalculated(ctx, event)
}

