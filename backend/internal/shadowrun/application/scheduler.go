package application

import (
	"context"
	"log"
	"time"
)

// Scheduler triggers shadowrun jobs on schedule.
type Scheduler struct {
	runner   *Runner
	tenantID string
	stations []string
	dailyAt  string
	logger   *log.Logger
}

// NewScheduler constructs a Scheduler.
func NewScheduler(runner *Runner, tenantID string, stations []string, dailyAt string, logger *log.Logger) *Scheduler {
	return &Scheduler{
		runner:   runner,
		tenantID: tenantID,
		stations: stations,
		dailyAt:  dailyAt,
		logger:   logger,
	}
}

// Start begins the scheduler loop.
func (s *Scheduler) Start(ctx context.Context) {
	if s == nil || s.runner == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if !s.shouldRun(now.UTC()) {
				continue
			}
			s.runOnce(ctx, now.UTC())
		}
	}
}

func (s *Scheduler) shouldRun(now time.Time) bool {
	hour, minute, err := parseDailyAt(s.dailyAt)
	if err != nil {
		return false
	}
	return now.Hour() == hour && now.Minute() == minute
}

func (s *Scheduler) runOnce(ctx context.Context, now time.Time) {
	if len(s.stations) == 0 {
		return
	}
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	jobDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for _, stationID := range s.stations {
		if stationID == "" {
			continue
		}
		if _, err := s.runner.Run(ctx, s.tenantID, stationID, month, jobDate, nil); err != nil && s.logger != nil {
			s.logger.Printf("shadowrun schedule error: station=%s err=%v", stationID, err)
		}
	}
}

func parseDailyAt(value string) (int, int, error) {
	t, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, err
	}
	return t.Hour(), t.Minute(), nil
}
