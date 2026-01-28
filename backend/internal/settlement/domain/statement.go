package settlement

import "time"

const (
	StatementStatusDraft  = "draft"
	StatementStatusFrozen = "frozen"
	StatementStatusVoided = "voided"
)

// StatementAggregate represents a monthly settlement statement.
type StatementAggregate struct {
	ID             string
	TenantID       string
	StationID      string
	StatementMonth time.Time
	Category       string
	Status         string
	Version        int
	TotalEnergyKWh float64
	TotalAmount    float64
	Currency       string
	SnapshotHash   string
	VoidReason     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	FrozenAt       time.Time
	VoidedAt       time.Time
}

// StatementItem represents a daily item in a statement.
type StatementItem struct {
	StatementID string
	DayStart    time.Time
	EnergyKWh   float64
	Amount      float64
	Currency    string
	CreatedAt   time.Time
}
