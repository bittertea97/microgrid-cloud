package metrics

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricPrefix = "platform_"

	resultSuccess = "success"
	resultError   = "error"

	commandResultAcked   = "acked"
	commandResultFailed  = "failed"
	commandResultTimeout = "timeout"
)

var (
	registerOnce sync.Once

	ingestRequests *prometheus.CounterVec
	ingestErrors   *prometheus.CounterVec
	ingestLatency  *prometheus.HistogramVec

	consumerLag *prometheus.GaugeVec

	commandRequests prometheus.Counter
	commandResults  *prometheus.CounterVec

	statementGenerateTotal   *prometheus.CounterVec
	statementGenerateLatency *prometheus.HistogramVec
	statementFreezeTotal     *prometheus.CounterVec
	statementFreezeLatency   *prometheus.HistogramVec
	statementExportTotal     *prometheus.CounterVec
	statementExportLatency   *prometheus.HistogramVec

	analyticsWindowTotal   *prometheus.CounterVec
	analyticsWindowLatency *prometheus.HistogramVec

	settlementDayTotal   *prometheus.CounterVec
	settlementDayLatency *prometheus.HistogramVec

	alarmEventsTotal *prometheus.CounterVec
)

// Init registers observability metrics and DB-backed gauges.
func Init(db *sql.DB, logger *log.Logger) {
	registerOnce.Do(func() {
		ingestRequests = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metricPrefix + "ingest_requests_total",
				Help: "Total ingest requests by result",
			},
			[]string{"result"},
		)
		ingestErrors = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metricPrefix + "ingest_errors_total",
				Help: "Total ingest errors by reason",
			},
			[]string{"reason"},
		)
		ingestLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    metricPrefix + "ingest_latency_seconds",
				Help:    "Ingest latency in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"result"},
		)

		consumerLag = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: metricPrefix + "event_consumer_lag_seconds",
				Help: "Consumer processing lag in seconds",
			},
			[]string{"consumer"},
		)

		commandRequests = prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: metricPrefix + "command_requests_total",
				Help: "Total issued commands",
			},
		)
		commandResults = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metricPrefix + "command_results_total",
				Help: "Total command results by status",
			},
			[]string{"status"},
		)

		statementGenerateTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metricPrefix + "statement_generate_total",
				Help: "Total statement generate operations by result",
			},
			[]string{"result"},
		)
		statementGenerateLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    metricPrefix + "statement_generate_latency_seconds",
				Help:    "Statement generate latency in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"result"},
		)
		statementFreezeTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metricPrefix + "statement_freeze_total",
				Help: "Total statement freeze operations by result",
			},
			[]string{"result"},
		)
		statementFreezeLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    metricPrefix + "statement_freeze_latency_seconds",
				Help:    "Statement freeze latency in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"result"},
		)
		statementExportTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metricPrefix + "statement_export_total",
				Help: "Total statement export operations by format and result",
			},
			[]string{"format", "result"},
		)
		statementExportLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    metricPrefix + "statement_export_latency_seconds",
				Help:    "Statement export latency in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"format", "result"},
		)

		analyticsWindowTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metricPrefix + "analytics_window_total",
				Help: "Total analytics hourly window calculations by result",
			},
			[]string{"result"},
		)
		analyticsWindowLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    metricPrefix + "analytics_window_latency_seconds",
				Help:    "Analytics hourly window latency in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"result"},
		)

		settlementDayTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metricPrefix + "settlement_day_total",
				Help: "Total day settlement calculations by result",
			},
			[]string{"result"},
		)
		settlementDayLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    metricPrefix + "settlement_day_latency_seconds",
				Help:    "Day settlement latency in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"result"},
		)

		alarmEventsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metricPrefix + "alarm_events_total",
				Help: "Total alarm lifecycle events by type",
			},
			[]string{"event"},
		)

		prometheus.MustRegister(
			ingestRequests,
			ingestErrors,
			ingestLatency,
			consumerLag,
			commandRequests,
			commandResults,
			statementGenerateTotal,
			statementGenerateLatency,
			statementFreezeTotal,
			statementFreezeLatency,
			statementExportTotal,
			statementExportLatency,
			analyticsWindowTotal,
			analyticsWindowLatency,
			settlementDayTotal,
			settlementDayLatency,
			alarmEventsTotal,
		)

		if db != nil {
			registerDBMetrics(db, logger)
		}
	})
}

// ObserveIngest records ingest request duration and result.
func ObserveIngest(result string, duration time.Duration) {
	if result == "" {
		result = resultSuccess
	}
	if ingestRequests != nil {
		ingestRequests.WithLabelValues(result).Inc()
	}
	if ingestLatency != nil {
		ingestLatency.WithLabelValues(result).Observe(duration.Seconds())
	}
}

// IncIngestError increments ingest error counter.
func IncIngestError(reason string) {
	if reason == "" {
		reason = "unknown"
	}
	if ingestErrors != nil {
		ingestErrors.WithLabelValues(reason).Inc()
	}
}

// ObserveConsumerLag sets consumer lag in seconds.
func ObserveConsumerLag(consumer string, lag time.Duration) {
	if consumer == "" {
		consumer = "unknown"
	}
	if lag < 0 {
		lag = 0
	}
	if consumerLag != nil {
		consumerLag.WithLabelValues(consumer).Set(lag.Seconds())
	}
}

// IncCommandIssued increments issued command counter.
func IncCommandIssued() {
	if commandRequests != nil {
		commandRequests.Inc()
	}
}

// IncCommandResult increments command result counter.
func IncCommandResult(status string) {
	if status == "" {
		status = "unknown"
	}
	if commandResults != nil {
		commandResults.WithLabelValues(status).Inc()
	}
}

// AddCommandTimeouts increments timeout counter by count.
func AddCommandTimeouts(count int) {
	if count <= 0 {
		return
	}
	if commandResults != nil {
		commandResults.WithLabelValues(commandResultTimeout).Add(float64(count))
	}
}

// ObserveStatementGenerate records generate latency and result.
func ObserveStatementGenerate(result string, duration time.Duration) {
	if result == "" {
		result = resultSuccess
	}
	if statementGenerateTotal != nil {
		statementGenerateTotal.WithLabelValues(result).Inc()
	}
	if statementGenerateLatency != nil {
		statementGenerateLatency.WithLabelValues(result).Observe(duration.Seconds())
	}
}

// ObserveStatementFreeze records freeze latency and result.
func ObserveStatementFreeze(result string, duration time.Duration) {
	if result == "" {
		result = resultSuccess
	}
	if statementFreezeTotal != nil {
		statementFreezeTotal.WithLabelValues(result).Inc()
	}
	if statementFreezeLatency != nil {
		statementFreezeLatency.WithLabelValues(result).Observe(duration.Seconds())
	}
}

// ObserveStatementExport records export latency and result.
func ObserveStatementExport(format, result string, duration time.Duration) {
	if format == "" {
		format = "unknown"
	}
	if result == "" {
		result = resultSuccess
	}
	if statementExportTotal != nil {
		statementExportTotal.WithLabelValues(format, result).Inc()
	}
	if statementExportLatency != nil {
		statementExportLatency.WithLabelValues(format, result).Observe(duration.Seconds())
	}
}

// ObserveAnalyticsWindow records analytics window latency and result.
func ObserveAnalyticsWindow(result string, duration time.Duration) {
	if result == "" {
		result = resultSuccess
	}
	if analyticsWindowTotal != nil {
		analyticsWindowTotal.WithLabelValues(result).Inc()
	}
	if analyticsWindowLatency != nil {
		analyticsWindowLatency.WithLabelValues(result).Observe(duration.Seconds())
	}
}

// ObserveSettlementDay records settlement calculation latency and result.
func ObserveSettlementDay(result string, duration time.Duration) {
	if result == "" {
		result = resultSuccess
	}
	if settlementDayTotal != nil {
		settlementDayTotal.WithLabelValues(result).Inc()
	}
	if settlementDayLatency != nil {
		settlementDayLatency.WithLabelValues(result).Observe(duration.Seconds())
	}
}

// IncAlarmEvent increments alarm lifecycle counters.
func IncAlarmEvent(event string) {
	if event == "" {
		event = "unknown"
	}
	if alarmEventsTotal != nil {
		alarmEventsTotal.WithLabelValues(event).Inc()
	}
}

// Exported constants for callers.
const (
	IngestResultSuccess = resultSuccess
	IngestResultError   = resultError

	ResultSuccess = resultSuccess
	ResultError   = resultError

	CommandResultAcked   = commandResultAcked
	CommandResultFailed  = commandResultFailed
	CommandResultTimeout = commandResultTimeout
)
