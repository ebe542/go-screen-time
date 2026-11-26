package sessionobserver

import (
	"context"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

// RunMonitor delivers observations in event order until the event stream
// closes, the context is cancelled, or the receiver returns an error.
func RunMonitor(
	ctx context.Context,
	monitor *Monitor,
	events <-chan MonitorEvent,
	emit func(tracking.Observation, error) error,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-events:
			if err := ctx.Err(); err != nil {
				return err
			}
			if !ok {
				return nil
			}

			observation, shouldEmit, readErr := monitor.Handle(event)
			if shouldEmit {
				if err := emit(observation, readErr); err != nil {
					return err
				}
			}
		}
	}
}
