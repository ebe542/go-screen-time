package sessionobserver

// SessionEventKind identifies a change to a session's activity or lock state.
type SessionEventKind uint8

const (
	SessionActivated SessionEventKind = iota + 1
	SessionDeactivated
	SessionLocked
	SessionUnlocked
)

// SessionEvent reports a state change for one session.
type SessionEvent struct {
	SessionID string
	Kind      SessionEventKind
}

type sessionFact uint8

const (
	factUnknown sessionFact = iota
	factYes
	factNo
)

// SessionTracker maintains the state of one interactive session.
type SessionTracker struct {
	id       string
	sid      string
	active   sessionFact
	unlocked sessionFact
}

// NewSessionTracker starts with unknown activity and lock state.
func NewSessionTracker(id, sid string) *SessionTracker {
	return &SessionTracker{id: id, sid: sid}
}

// Apply updates the state for events belonging to this session.
func (s *SessionTracker) Apply(event SessionEvent) {
	if event.SessionID != s.id {
		return
	}

	switch event.Kind {
	case SessionActivated:
		s.active = factYes
	case SessionDeactivated:
		s.active = factNo
	case SessionLocked:
		s.unlocked = factNo
	case SessionUnlocked:
		s.unlocked = factYes
	}
}

// Snapshot returns the session's current observation state.
func (s *SessionTracker) Snapshot() SessionSnapshot {
	state := SessionUnknown
	switch {
	case s.active == factNo || s.unlocked == factNo:
		state = SessionInactive
	case s.active == factYes && s.unlocked == factYes:
		state = SessionActive
	}

	return SessionSnapshot{ID: s.id, SID: s.sid, State: state}
}
