package metrics

import (
	"database/sql"
	"log"

	"github.com/prometheus/client_golang/prometheus"
)

func registerDBMetrics(db *sql.DB, logger *log.Logger) {
	prometheus.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: metricPrefix + "event_outbox_pending",
			Help: "Pending outbox records",
		},
		func() float64 {
			return queryCount(db, logger, "SELECT COUNT(*) FROM event_outbox WHERE status = 'pending'")
		},
	))

	prometheus.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: metricPrefix + "event_dlq_count",
			Help: "Dead letter queue records",
		},
		func() float64 {
			return queryCount(db, logger, "SELECT COUNT(*) FROM dead_letter_events")
		},
	))
}

func queryCount(db *sql.DB, logger *log.Logger, query string) float64 {
	if db == nil {
		return 0
	}
	var count int64
	if err := db.QueryRow(query).Scan(&count); err != nil {
		if logger != nil {
			logger.Printf("metrics query failed: %v", err)
		}
		return 0
	}
	if count < 0 {
		return 0
	}
	return float64(count)
}
