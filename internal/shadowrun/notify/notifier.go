package notify

import "context"

// AlertMessage represents a notification payload.
type AlertMessage struct {
	TenantID          string            `json:"tenant_id"`
	StationID         string            `json:"station_id"`
	Month             string            `json:"month"`
	ReportID          string            `json:"report_id"`
	ReportURL         string            `json:"report_url"`
	DiffSummary       map[string]any    `json:"diff_summary"`
	RecommendedAction string            `json:"recommended_action"`
	Meta              map[string]string `json:"meta,omitempty"`
}

// Notifier sends notifications.
type Notifier interface {
	Notify(ctx context.Context, msg AlertMessage) error
}
