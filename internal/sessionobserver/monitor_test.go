package sessionobserver

import (
	"errors"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestMonitorEmitsOrderedSessionObservations(t *testing.T) {
	const id = "12"
	const sid = "S-1-5-21-100-200-300-1001"
	const game = `C:\Games\Example\game.exe`
	const writer = `C:\School\writer.exe`

	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	session := NewSessionTracker(id, sid)
	session.Apply(SessionEvent{SessionID: id, Kind: SessionActivated})
	session.Apply(SessionEvent{SessionID: id, Kind: SessionUnlocked})

	path := game
	calls := 0
	source := foregroundSourceFunc(func() (ForegroundProcess, error) {
		calls++
		return ForegroundProcess{
			SessionID:      id,
			SID:            sid,
			ExecutablePath: path,
		}, nil
	})
	monitor := NewMonitor(session, source)

	tests := []struct {
		name      string
		offset    time.Duration
		kind      MonitorEventKind
		session   SessionEvent
		path      string
		wantState tracking.ObservationState
		wantPath  string
		wantEmit  bool
		wantCalls int
	}{
		{
			name: "start in active session", kind: MonitorStart, path: game,
			wantState: tracking.ObservationForeground, wantPath: game,
			wantEmit: true, wantCalls: 1,
		},
		{
			name: "foreground changes before next confirmation", offset: 2 * time.Second,
			kind: MonitorForegroundChanged, path: writer,
			wantState: tracking.ObservationForeground, wantPath: writer,
			wantEmit: true, wantCalls: 2,
		},
		{
			name: "periodic confirmation", offset: 5 * time.Second,
			kind: MonitorHeartbeat, path: writer,
			wantState: tracking.ObservationForeground, wantPath: writer,
			wantEmit: true, wantCalls: 3,
		},
		{
			name: "foreign session lock is ignored", offset: 6 * time.Second,
			kind: MonitorSessionChanged, path: writer,
			session:   SessionEvent{SessionID: "13", Kind: SessionLocked},
			wantCalls: 3,
		},
		{
			name: "own session locks", offset: 7 * time.Second,
			kind: MonitorSessionChanged, path: writer,
			session:   SessionEvent{SessionID: id, Kind: SessionLocked},
			wantState: tracking.ObservationInactive, wantEmit: true, wantCalls: 3,
		},
		{
			name: "heartbeat while locked", offset: 10 * time.Second,
			kind: MonitorHeartbeat, path: game,
			wantState: tracking.ObservationInactive, wantEmit: true, wantCalls: 3,
		},
		{
			name: "unlock reads fresh foreground", offset: 11 * time.Second,
			kind: MonitorSessionChanged, path: game,
			session:   SessionEvent{SessionID: id, Kind: SessionUnlocked},
			wantState: tracking.ObservationForeground, wantPath: game,
			wantEmit: true, wantCalls: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path = tt.path
			at := start.Add(tt.offset)
			got, emit, err := monitor.Handle(MonitorEvent{
				At: at, Kind: tt.kind, Session: tt.session,
			})
			if err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if emit != tt.wantEmit {
				t.Errorf("emit = %t, want %t", emit, tt.wantEmit)
			}
			if calls != tt.wantCalls {
				t.Errorf("foreground queries = %d, want %d", calls, tt.wantCalls)
			}
			if !tt.wantEmit {
				return
			}

			want := tracking.Observation{
				At:             at,
				SessionID:      id,
				SID:            sid,
				ExecutablePath: tt.wantPath,
				State:          tt.wantState,
			}
			if got != want {
				t.Errorf("observation = %+v, want %+v", got, want)
			}
		})
	}
}

func TestMonitorEmitsUnavailableOnUnknownStateAndReadError(t *testing.T) {
	const id = "12"
	const sid = "S-1-5-21-100-200-300-1001"

	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	session := NewSessionTracker(id, sid)
	readErr := errors.New("foreground process unavailable")
	calls := 0
	source := foregroundSourceFunc(func() (ForegroundProcess, error) {
		calls++
		return ForegroundProcess{}, readErr
	})
	monitor := NewMonitor(session, source)

	got, emit, err := monitor.Handle(MonitorEvent{At: at, Kind: MonitorStart})
	if !emit || got.State != tracking.ObservationUnavailable || err != nil || calls != 0 {
		t.Fatalf("unknown start: observation = %+v, emit = %t, err = %v, calls = %d",
			got, emit, err, calls)
	}

	session.Apply(SessionEvent{SessionID: id, Kind: SessionActivated})
	session.Apply(SessionEvent{SessionID: id, Kind: SessionUnlocked})
	next := at.Add(5 * time.Second)
	got, emit, err = monitor.Handle(MonitorEvent{At: next, Kind: MonitorHeartbeat})

	want := tracking.Observation{
		At: next, SessionID: id, SID: sid, State: tracking.ObservationUnavailable,
	}
	if !emit || got != want || !errors.Is(err, readErr) || calls != 1 {
		t.Errorf("failed read: observation = %+v, emit = %t, err = %v, calls = %d",
			got, emit, err, calls)
	}
}
