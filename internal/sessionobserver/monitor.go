package sessionobserver

import (
	"time"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

// MonitorEventKind identifies what triggered a new observation.
type MonitorEventKind uint8

const (
	MonitorStart MonitorEventKind = iota + 1
	MonitorForegroundChanged
	MonitorHeartbeat
	MonitorSessionChanged
)

// MonitorEvent carries the observation time and any session change.
type MonitorEvent struct {
	At      time.Time
	Kind    MonitorEventKind
	Session SessionEvent
}

// Monitor turns ordered events from one session into tracking observations.
// Its caller processes events sequentially.
type Monitor struct {
	session *SessionTracker
	source  ForegroundSource
}

// NewMonitor creates a monitor for the given interactive session.
func NewMonitor(session *SessionTracker, source ForegroundSource) *Monitor {
	return &Monitor{session: session, source: source}
}

// Handle processes one event. A true second result means the observation
// should be emitted even when a foreground read error is also returned.
func (m *Monitor) Handle(event MonitorEvent) (tracking.Observation, bool, error) {
	switch event.Kind {
	case MonitorSessionChanged:
		if event.Session.SessionID != m.session.id {
			return tracking.Observation{}, false, nil
		}
		m.session.Apply(event.Session)
	case MonitorStart, MonitorForegroundChanged, MonitorHeartbeat:
		// These events sample the current session state.
	default:
		return tracking.Observation{}, false, nil
	}

	observation, err := CaptureObservation(m.source, m.session.Snapshot(), event.At)
	return observation, true, err
}
