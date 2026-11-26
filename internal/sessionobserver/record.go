package sessionobserver

import (
	"context"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

// RecordMonitorUsage forwards monitor observations to the usage tracker.
// reportReadError, when provided, receives foreground read errors.
func RecordMonitorUsage(
	ctx context.Context,
	monitor *Monitor,
	events <-chan MonitorEvent,
	tracker *tracking.Tracker,
	reportReadError func(error),
) error {
	return RunMonitor(ctx, monitor, events, func(
		observation tracking.Observation,
		readErr error,
	) error {
		if readErr != nil && reportReadError != nil {
			reportReadError(readErr)
		}
		return tracker.Observe(observation)
	})
}
