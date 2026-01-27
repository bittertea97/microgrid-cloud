package notify

import (
	"context"

	alarmapp "microgrid-cloud/internal/alarms/application"
)

// MultiNotifier dispatches alarm events to multiple notifiers.
type MultiNotifier struct {
	notifiers []alarmapp.AlarmNotifier
}

// NewMultiNotifier constructs a MultiNotifier.
func NewMultiNotifier(notifiers ...alarmapp.AlarmNotifier) *MultiNotifier {
	return &MultiNotifier{notifiers: notifiers}
}

// Notify forwards events to all notifiers.
func (m *MultiNotifier) Notify(ctx context.Context, event alarmapp.AlarmEvent) {
	if m == nil {
		return
	}
	for _, notifier := range m.notifiers {
		if notifier != nil {
			notifier.Notify(ctx, event)
		}
	}
}
