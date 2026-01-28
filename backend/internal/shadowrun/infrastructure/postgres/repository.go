package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Job represents a shadowrun job.
type Job struct {
	ID        string
	TenantID  string
	StationID string
	Month     time.Time
	JobDate   time.Time
	JobType   string
	Status    string
	Attempts  int
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
	StartedAt *time.Time
	EndedAt   *time.Time
}

// Report represents a shadowrun report.
type Report struct {
	ID                string
	JobID             string
	TenantID          string
	StationID         string
	Month             time.Time
	ReportDate        time.Time
	Status            string
	Location          string
	DiffSummary       []byte
	DiffEnergyKWhMax  float64
	DiffAmountMax     float64
	MissingHours      int
	RecommendedAction string
	CreatedAt         time.Time
}

// ShadowrunAlert represents a shadowrun alert record.
type ShadowrunAlert struct {
	ID        string
	TenantID  string
	StationID string
	Category  string
	Severity  string
	Title     string
	Message   string
	Payload   []byte
	ReportID  string
	Status    string
	CreatedAt time.Time
}

// Repository handles shadowrun persistence.
type Repository struct {
	db *sql.DB
}

// NewRepository constructs a repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CreateJob inserts a job if not exists, then returns the stored job.
func (r *Repository) CreateJob(ctx context.Context, job *Job) (*Job, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("shadowrun repo: nil db")
	}
	if job == nil {
		return nil, errors.New("shadowrun repo: nil job")
	}
	now := time.Now().UTC()
	_, _ = r.db.ExecContext(ctx, `
INSERT INTO shadowrun_jobs (
	id, tenant_id, station_id, month, job_date, job_type, status, attempts, created_at, updated_at
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,0,$8,$8
)
ON CONFLICT (tenant_id, station_id, month, job_date, job_type)
DO NOTHING`,
		job.ID, job.TenantID, job.StationID, job.Month, job.JobDate, job.JobType, job.Status, now,
	)
	return r.GetJobByKey(ctx, job.TenantID, job.StationID, job.Month, job.JobDate, job.JobType)
}

// GetJobByKey returns job by unique key.
func (r *Repository) GetJobByKey(ctx context.Context, tenantID, stationID string, month, jobDate time.Time, jobType string) (*Job, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, tenant_id, station_id, month, job_date, job_type, status, attempts, error, created_at, updated_at, started_at, finished_at
FROM shadowrun_jobs
WHERE tenant_id = $1 AND station_id = $2 AND month = $3 AND job_date = $4 AND job_type = $5`,
		tenantID, stationID, month, jobDate, jobType)

	return scanJob(row)
}

// UpdateJobStatus updates job status and timestamps.
func (r *Repository) UpdateJobStatus(ctx context.Context, id, status, errMsg string, startedAt, finishedAt *time.Time, bumpAttempt bool) error {
	if r == nil || r.db == nil {
		return errors.New("shadowrun repo: nil db")
	}
	if id == "" {
		return errors.New("shadowrun repo: empty job id")
	}
	now := time.Now().UTC()
	if bumpAttempt {
		_, err := r.db.ExecContext(ctx, `
UPDATE shadowrun_jobs
SET status = $1, error = $2, started_at = $3, finished_at = $4, attempts = attempts + 1, updated_at = $5
WHERE id = $6`, status, errMsg, startedAt, finishedAt, now, id)
		return err
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE shadowrun_jobs
SET status = $1, error = $2, started_at = $3, finished_at = $4, updated_at = $5
WHERE id = $6`, status, errMsg, startedAt, finishedAt, now, id)
	return err
}

// CreateReport inserts a report.
func (r *Repository) CreateReport(ctx context.Context, report *Report) error {
	if r == nil || r.db == nil {
		return errors.New("shadowrun repo: nil db")
	}
	if report == nil {
		return errors.New("shadowrun repo: nil report")
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
INSERT INTO shadowrun_reports (
	id, job_id, tenant_id, station_id, month, report_date, status, report_location,
	diff_summary, diff_energy_kwh_max, diff_amount_max, missing_hours, recommended_action, created_at
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
)`,
		report.ID, report.JobID, report.TenantID, report.StationID, report.Month, report.ReportDate, report.Status, report.Location,
		report.DiffSummary, report.DiffEnergyKWhMax, report.DiffAmountMax, report.MissingHours, report.RecommendedAction, now)
	return err
}

// ListReports lists reports for a station and time range.
func (r *Repository) ListReports(ctx context.Context, stationID string, from, to time.Time) ([]Report, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("shadowrun repo: nil db")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, job_id, tenant_id, station_id, month, report_date, status, report_location,
	diff_summary, diff_energy_kwh_max, diff_amount_max, missing_hours, recommended_action, created_at
FROM shadowrun_reports
WHERE station_id = $1 AND report_date >= $2 AND report_date < $3
ORDER BY report_date DESC`, stationID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Report
	for rows.Next() {
		var report Report
		if err := rows.Scan(
			&report.ID,
			&report.JobID,
			&report.TenantID,
			&report.StationID,
			&report.Month,
			&report.ReportDate,
			&report.Status,
			&report.Location,
			&report.DiffSummary,
			&report.DiffEnergyKWhMax,
			&report.DiffAmountMax,
			&report.MissingHours,
			&report.RecommendedAction,
			&report.CreatedAt,
		); err != nil {
			return nil, err
		}
		report.Month = report.Month.UTC()
		report.ReportDate = report.ReportDate.UTC()
		report.CreatedAt = report.CreatedAt.UTC()
		result = append(result, report)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetReport returns report by id.
func (r *Repository) GetReport(ctx context.Context, id string) (*Report, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("shadowrun repo: nil db")
	}
	row := r.db.QueryRowContext(ctx, `
SELECT id, job_id, tenant_id, station_id, month, report_date, status, report_location,
	diff_summary, diff_energy_kwh_max, diff_amount_max, missing_hours, recommended_action, created_at
FROM shadowrun_reports
WHERE id = $1`, id)

	var report Report
	if err := row.Scan(
		&report.ID,
		&report.JobID,
		&report.TenantID,
		&report.StationID,
		&report.Month,
		&report.ReportDate,
		&report.Status,
		&report.Location,
		&report.DiffSummary,
		&report.DiffEnergyKWhMax,
		&report.DiffAmountMax,
		&report.MissingHours,
		&report.RecommendedAction,
		&report.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	report.Month = report.Month.UTC()
	report.ReportDate = report.ReportDate.UTC()
	report.CreatedAt = report.CreatedAt.UTC()
	return &report, nil
}

// CreateSystemAlert inserts a shadowrun alert (legacy naming kept for compatibility).
func (r *Repository) CreateSystemAlert(ctx context.Context, alert *ShadowrunAlert) error {
	if r == nil || r.db == nil {
		return errors.New("shadowrun repo: nil db")
	}
	if alert == nil {
		return errors.New("shadowrun repo: nil alert")
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
INSERT INTO shadowrun_alerts (
	id, tenant_id, station_id, category, severity, title, message, payload, report_id, status, created_at
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11
)`,
		alert.ID, alert.TenantID, alert.StationID, alert.Category, alert.Severity, alert.Title, alert.Message, alert.Payload, alert.ReportID, alert.Status, now)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (*Job, error) {
	var job Job
	var started sql.NullTime
	var finished sql.NullTime
	var errMsg sql.NullString
	if err := row.Scan(
		&job.ID,
		&job.TenantID,
		&job.StationID,
		&job.Month,
		&job.JobDate,
		&job.JobType,
		&job.Status,
		&job.Attempts,
		&errMsg,
		&job.CreatedAt,
		&job.UpdatedAt,
		&started,
		&finished,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if errMsg.Valid {
		job.Error = errMsg.String
	}
	if started.Valid {
		t := started.Time.UTC()
		job.StartedAt = &t
	}
	if finished.Valid {
		t := finished.Time.UTC()
		job.EndedAt = &t
	}
	job.Month = job.Month.UTC()
	job.JobDate = job.JobDate.UTC()
	job.CreatedAt = job.CreatedAt.UTC()
	job.UpdatedAt = job.UpdatedAt.UTC()
	return &job, nil
}
