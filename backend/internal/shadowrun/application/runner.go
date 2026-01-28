package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"time"

	shadowrepo "microgrid-cloud/internal/shadowrun/infrastructure/postgres"
	shadowmetrics "microgrid-cloud/internal/shadowrun/metrics"
	shadownotify "microgrid-cloud/internal/shadowrun/notify"
)

const (
	jobTypeShadowrun = "shadowrun"
	jobStatusCreated = "created"
	jobStatusRunning = "running"
	jobStatusSuccess = "succeeded"
	jobStatusFailed  = "failed"
)

// Runner executes shadowrun jobs.
type Runner struct {
	repo          *shadowrepo.Repository
	db            *sql.DB
	thresholds    Config
	notifier      shadownotify.Notifier
	metrics       *shadowmetrics.Metrics
	logger        *log.Logger
	publicBaseURL string
	storageRoot   string
	fallbackPrice float64
}

// NewRunner constructs a Runner.
func NewRunner(repo *shadowrepo.Repository, db *sql.DB, cfg Config, notifier shadownotify.Notifier, metrics *shadowmetrics.Metrics, logger *log.Logger) *Runner {
	return &Runner{
		repo:          repo,
		db:            db,
		thresholds:    cfg,
		notifier:      notifier,
		metrics:       metrics,
		logger:        logger,
		publicBaseURL: cfg.PublicBaseURL,
		storageRoot:   cfg.StorageRoot,
		fallbackPrice: cfg.FallbackPrice,
	}
}

// Run executes a shadowrun job for a station/month.
func (r *Runner) Run(ctx context.Context, tenantID, stationID string, month time.Time, jobDate time.Time, override *Thresholds) (*shadowrepo.Report, error) {
	if r == nil {
		return nil, fmt.Errorf("shadowrun runner: nil")
	}
	if tenantID == "" || stationID == "" {
		return nil, fmt.Errorf("shadowrun runner: tenant_id/station_id required")
	}
	jobDate = time.Date(jobDate.Year(), jobDate.Month(), jobDate.Day(), 0, 0, 0, 0, time.UTC)
	monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)
	if err := validateMonth(monthStart, monthEnd); err != nil {
		return nil, err
	}

	jobID := fmt.Sprintf("sr-%s-%s-%s", stationID, monthStart.Format("200601"), jobDate.Format("20060102"))
	job, err := r.repo.CreateJob(ctx, &shadowrepo.Job{
		ID:        jobID,
		TenantID:  tenantID,
		StationID: stationID,
		Month:     monthStart,
		JobDate:   jobDate,
		JobType:   jobTypeShadowrun,
		Status:    jobStatusCreated,
	})
	if err != nil {
		return nil, err
	}
	if job.Status == jobStatusSuccess {
		report, _ := r.repo.GetReport(ctx, job.ID)
		return report, nil
	}
	if job.Status == jobStatusRunning {
		return nil, fmt.Errorf("shadowrun job already running")
	}

	started := time.Now().UTC()
	_ = r.repo.UpdateJobStatus(ctx, job.ID, jobStatusRunning, "", &started, nil, true)
	if r.metrics != nil {
		r.metrics.JobsTotal.WithLabelValues(jobStatusRunning).Inc()
	}
	r.logf("shadowrun_job_start", tenantID, stationID, job.ID, "", "")

	thresholds := r.thresholds.ThresholdsForStation(stationID)
	if override != nil {
		thresholds = mergeThresholds(thresholds, *override)
	}

	result, _, _, err := reconcile(ctx, r.db, tenantID, stationID, monthStart, monthEnd, r.fallbackPrice)
	if err != nil {
		ended := time.Now().UTC()
		_ = r.repo.UpdateJobStatus(ctx, job.ID, jobStatusFailed, err.Error(), &started, &ended, false)
		if r.metrics != nil {
			r.metrics.JobsTotal.WithLabelValues(jobStatusFailed).Inc()
		}
		r.logf("shadowrun_job_failed", tenantID, stationID, job.ID, "", err.Error())
		return nil, err
	}

	reportDir := filepath.Join(r.storageRoot, tenantID, stationID, monthStart.Format("2006-01"), job.ID)
	if err := writeReports(reportDir, result); err != nil {
		ended := time.Now().UTC()
		_ = r.repo.UpdateJobStatus(ctx, job.ID, jobStatusFailed, err.Error(), &started, &ended, false)
		if r.metrics != nil {
			r.metrics.JobsTotal.WithLabelValues(jobStatusFailed).Inc()
		}
		r.logf("shadowrun_job_failed", tenantID, stationID, job.ID, "", err.Error())
		return nil, err
	}

	summary, err := buildDiffSummary(result, monthStart, monthEnd, jobDate, thresholds)
	if err != nil {
		ended := time.Now().UTC()
		_ = r.repo.UpdateJobStatus(ctx, job.ID, jobStatusFailed, err.Error(), &started, &ended, false)
		if r.metrics != nil {
			r.metrics.JobsTotal.WithLabelValues(jobStatusFailed).Inc()
		}
		return nil, err
	}
	_ = writeSummaryJSON(reportDir, summary)
	archivePath, err := writeArchive(reportDir)
	if err != nil {
		ended := time.Now().UTC()
		_ = r.repo.UpdateJobStatus(ctx, job.ID, jobStatusFailed, err.Error(), &started, &ended, false)
		return nil, err
	}

	recommended := recommendedAction(summary, thresholds)
	summaryBytes, _ := json.Marshal(summary)
	reportID := "report-" + job.ID

	report := &shadowrepo.Report{
		ID:                reportID,
		JobID:             job.ID,
		TenantID:          tenantID,
		StationID:         stationID,
		Month:             monthStart,
		ReportDate:        jobDate,
		Status:            "generated",
		Location:          archivePath,
		DiffSummary:       summaryBytes,
		DiffEnergyKWhMax:  summary.DiffEnergyMax,
		DiffAmountMax:     summary.DiffAmountMax,
		MissingHours:      summary.MissingHoursTotal,
		RecommendedAction: recommended,
		CreatedAt:         time.Now().UTC(),
	}

	if err := r.repo.CreateReport(ctx, report); err != nil {
		ended := time.Now().UTC()
		_ = r.repo.UpdateJobStatus(ctx, job.ID, jobStatusFailed, err.Error(), &started, &ended, false)
		return nil, err
	}

	notifyNeeded := isThresholdExceeded(summary, thresholds)
	if notifyNeeded {
		if err := r.createAlert(ctx, report, summary, recommended); err != nil {
			r.logf("shadowrun_alert_failed", tenantID, stationID, job.ID, report.ID, err.Error())
		} else if r.metrics != nil {
			r.metrics.AlertsTotal.Inc()
		}
	}

	ended := time.Now().UTC()
	_ = r.repo.UpdateJobStatus(ctx, job.ID, jobStatusSuccess, "", &started, &ended, false)
	if r.metrics != nil {
		r.metrics.JobsTotal.WithLabelValues(jobStatusSuccess).Inc()
		r.metrics.JobDuration.Observe(ended.Sub(started).Seconds())
		r.metrics.ReportsTotal.Inc()
		r.metrics.DiffEnergyMax.Set(summary.DiffEnergyMax)
		r.metrics.DiffAmountMax.Set(summary.DiffAmountMax)
		r.metrics.DiffMax.Set(maxFloat(summary.DiffEnergyMax, summary.DiffAmountMax))
	}
	r.logf("shadowrun_job_success", tenantID, stationID, job.ID, report.ID, "")
	return report, nil
}

func (r *Runner) createAlert(ctx context.Context, report *shadowrepo.Report, summary diffSummary, recommended string) error {
	if report == nil {
		return nil
	}
	payload := map[string]any{
		"diff_energy_max":    summary.DiffEnergyMax,
		"diff_amount_max":    summary.DiffAmountMax,
		"missing_hours":      summary.MissingHoursTotal,
		"late_data_count":    summary.LateDataCount,
		"recommended_action": recommended,
	}
	payloadBytes, _ := json.Marshal(payload)
	alert := &shadowrepo.ShadowrunAlert{
		ID:        "alert-" + report.ID,
		TenantID:  report.TenantID,
		StationID: report.StationID,
		Category:  "shadowrun",
		Severity:  "high",
		Title:     fmt.Sprintf("Shadowrun diff alert: %s", report.StationID),
		Message:   fmt.Sprintf("Diff exceeds threshold for %s %s", report.StationID, summary.Month),
		Payload:   payloadBytes,
		ReportID:  report.ID,
		Status:    "open",
		CreatedAt: time.Now().UTC(),
	}
	if err := r.repo.CreateSystemAlert(ctx, alert); err != nil {
		return err
	}
	if r.notifier != nil {
		msg := shadownotify.AlertMessage{
			TenantID:          report.TenantID,
			StationID:         report.StationID,
			Month:             summary.Month,
			ReportID:          report.ID,
			ReportURL:         fmt.Sprintf("%s/api/v1/shadowrun/reports/%s/download", r.publicBaseURL, report.ID),
			DiffSummary:       payload,
			RecommendedAction: recommended,
			Meta:              map[string]string{"job_id": report.JobID},
		}
		return r.notifier.Notify(ctx, msg)
	}
	return nil
}

func isThresholdExceeded(summary diffSummary, thresholds Thresholds) bool {
	if thresholds.MissingHours > 0 && summary.MissingHoursTotal >= thresholds.MissingHours {
		return true
	}
	if thresholds.EnergyAbs > 0 && summary.DiffEnergyMax >= thresholds.EnergyAbs {
		return true
	}
	if thresholds.AmountAbs > 0 && summary.DiffAmountMax >= thresholds.AmountAbs {
		return true
	}
	return false
}

func recommendedAction(summary diffSummary, thresholds Thresholds) string {
	if thresholds.MissingHours > 0 && summary.MissingHoursTotal >= thresholds.MissingHours {
		return "replay_missing_hours"
	}
	if thresholds.EnergyAbs > 0 && summary.DiffEnergyMax >= thresholds.EnergyAbs {
		return "check_mapping_or_tariff"
	}
	if thresholds.AmountAbs > 0 && summary.DiffAmountMax >= thresholds.AmountAbs {
		return "check_tariff_or_settlement"
	}
	return "none"
}

func (r *Runner) logf(event, tenantID, stationID, jobID, reportID, errMsg string) {
	if r.logger == nil {
		return
	}
	r.logger.Printf("event=%s tenant_id=%s station_id=%s job_id=%s report_id=%s correlation_id=%s error=%s",
		event, tenantID, stationID, jobID, reportID, jobID, errMsg)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
