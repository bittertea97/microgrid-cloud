package alarms

import "errors"

// ErrNotFound indicates a missing alarm record.
var ErrNotFound = errors.New("alarm: not found")
