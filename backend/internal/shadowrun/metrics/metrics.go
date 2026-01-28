package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics bundles shadowrun metrics.
type Metrics struct {
	JobsTotal     *prometheus.CounterVec
	JobDuration   prometheus.Histogram
	DiffEnergyMax prometheus.Gauge
	DiffAmountMax prometheus.Gauge
	DiffMax       prometheus.Gauge
	ReportsTotal  prometheus.Counter
	AlertsTotal   prometheus.Counter
}

// New constructs and registers metrics.
func New() *Metrics {
	m := &Metrics{
		JobsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "platform_shadowrun_jobs_total",
				Help: "Total shadowrun jobs by status",
			},
			[]string{"status"},
		),
		JobDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "platform_shadowrun_job_duration_seconds",
			Help:    "Shadowrun job duration in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		DiffEnergyMax: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "platform_shadowrun_diff_energy_kwh_max",
			Help: "Max energy diff in kWh",
		}),
		DiffAmountMax: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "platform_shadowrun_diff_amount_max",
			Help: "Max amount diff",
		}),
		DiffMax: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "platform_shadowrun_diff_max",
			Help: "Max shadowrun diff (energy or amount)",
		}),
		ReportsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "platform_shadowrun_reports_total",
			Help: "Total shadowrun reports",
		}),
		AlertsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "platform_shadowrun_alerts_total",
			Help: "Total shadowrun alerts",
		}),
	}
	prometheus.MustRegister(
		m.JobsTotal,
		m.JobDuration,
		m.DiffEnergyMax,
		m.DiffAmountMax,
		m.DiffMax,
		m.ReportsTotal,
		m.AlertsTotal,
	)
	return m
}
