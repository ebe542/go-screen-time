package sessionobserver

import (
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestSessionTrackerAppliesOnlyEventsForItsOwnSession(t *testing.T) {
	const id = "session-a"
	const sid = "S-1-5-21-100-200-300-1001"

	session := NewSessionTracker(id, sid)
	assertSessionSnapshot(t, session, id, sid, SessionUnknown)

	steps := []struct {
		name  string
		event SessionEvent
		want  SessionState
	}{
		{"active but lock state unknown", SessionEvent{id, SessionActivated}, SessionUnknown},
		{"foreign unlock", SessionEvent{"session-b", SessionUnlocked}, SessionUnknown},
		{"own unlock", SessionEvent{id, SessionUnlocked}, SessionActive},
		{"own lock", SessionEvent{id, SessionLocked}, SessionInactive},
		{"foreign unlock while locked", SessionEvent{"session-b", SessionUnlocked}, SessionInactive},
		{"own unlock after lock", SessionEvent{id, SessionUnlocked}, SessionActive},
		{"deactivated", SessionEvent{id, SessionDeactivated}, SessionInactive},
		{"unlock while inactive", SessionEvent{id, SessionUnlocked}, SessionInactive},
		{"reactivated while unlocked", SessionEvent{id, SessionActivated}, SessionActive},
	}

	for _, step := range steps {
		session.Apply(step.event)
		t.Run(step.name, func(t *testing.T) {
			assertSessionSnapshot(t, session, id, sid, step.want)
		})
	}
}

func TestSessionTrackerCapturesFreshForegroundAfterUnlock(t *testing.T) {
	const id = "session-a"
	const sid = "S-1-5-21-100-200-300-1001"
	const firstPath = `C:\Games\A\game.exe`
	const secondPath = `C:\School\writer.exe`

	session := NewSessionTracker(id, sid)
	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	path := firstPath
	calls := 0
	source := foregroundSourceFunc(func() (ForegroundProcess, error) {
		calls++
		return ForegroundProcess{SessionID: id, SID: sid, ExecutablePath: path}, nil
	})

	steps := []struct {
		name      string
		event     SessionEventKind
		path      string
		wantState tracking.ObservationState
		wantPath  string
		wantCalls int
	}{
		{"unlocked but activation unknown", SessionUnlocked, firstPath, tracking.ObservationUnavailable, "", 0},
		{"active foreground", SessionActivated, firstPath, tracking.ObservationForeground, firstPath, 1},
		{"known lock", SessionLocked, secondPath, tracking.ObservationInactive, "", 1},
		{"fresh foreground after unlock", SessionUnlocked, secondPath, tracking.ObservationForeground, secondPath, 2},
	}

	for i, step := range steps {
		path = step.path
		session.Apply(SessionEvent{SessionID: id, Kind: step.event})
		observedAt := at.Add(time.Duration(i) * time.Second)

		t.Run(step.name, func(t *testing.T) {
			got, err := CaptureObservation(source, session.Snapshot(), observedAt)
			if err != nil {
				t.Fatalf("capture observation: %v", err)
			}
			want := tracking.Observation{
				At:             observedAt,
				SessionID:      id,
				SID:            sid,
				State:          step.wantState,
				ExecutablePath: step.wantPath,
			}
			if got != want {
				t.Errorf("observation = %+v, want %+v", got, want)
			}
			if calls != step.wantCalls {
				t.Errorf("foreground queries = %d, want %d", calls, step.wantCalls)
			}
		})
	}
}

func assertSessionSnapshot(t *testing.T, session *SessionTracker, id, sid string, state SessionState) {
	t.Helper()
	want := SessionSnapshot{ID: id, SID: sid, State: state}
	if got := session.Snapshot(); got != want {
		t.Errorf("snapshot = %+v, want %+v", got, want)
	}
}
