package strategy

import "time"

// Template defines a strategy template.
type Template struct {
	ID        string
	Type      string
	Params    []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}
