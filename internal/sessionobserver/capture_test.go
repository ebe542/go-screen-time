package sessionobserver

import (
	"errors"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestCaptureObservation(t *testing.T) {
	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	const sessionID = "session-a"
	const sid = "S-1-5-21-100-200-300-1001"
	const executable = `C:\Games\Example\game.exe`

	valid := ForegroundProcess{
		SessionID:      sessionID,
		SID:            sid,
		ExecutablePath: executable,
	}
	wrongSession := valid
	wrongSession.SessionID = "session-b"
	wrongAccount := valid
	wrongAccount.SID = "S-1-5-21-100-200-300-1002"
	missingPath := valid
	missingPath.ExecutablePath = ""

	pathErr := errors.New("read foreground process image path")

	tests := []struct {
		name         string
		sessionState SessionState
		process      ForegroundProcess
		sourceErr    error
		wantState    tracking.ObservationState
		wantPath     string
		wantCalls    int
	}{
		{
			name:         "confirmed foreground process",
			sessionState: SessionActive,
			process:      valid,
			wantState:    tracking.ObservationForeground,
			wantPath:     executable,
			wantCalls:    1,
		},
		{
			name:         "known session lock",
			sessionState: SessionInactive,
			wantState:    tracking.ObservationInactive,
		},
		{
			name:         "unknown session state",
			sessionState: SessionUnknown,
			wantState:    tracking.ObservationUnavailable,
		},
		{
			name:         "no foreground window",
			sessionState: SessionActive,
			wantState:    tracking.ObservationUnavailable,
			wantCalls:    1,
		},
		{
			name:         "foreground process has no image path",
			sessionState: SessionActive,
			process:      missingPath,
			wantState:    tracking.ObservationUnavailable,
			wantCalls:    1,
		},
		{
			name:         "image path query fails",
			sessionState: SessionActive,
			sourceErr:    pathErr,
			wantState:    tracking.ObservationUnavailable,
			wantCalls:    1,
		},
		{
			name:         "foreground process belongs to another session",
			sessionState: SessionActive,
			process:      wrongSession,
			wantState:    tracking.ObservationUnavailable,
			wantCalls:    1,
		},
		{
			name:         "foreground process belongs to another account",
			sessionState: SessionActive,
			process:      wrongAccount,
			wantState:    tracking.ObservationUnavailable,
			wantCalls:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			source := foregroundSourceFunc(func() (ForegroundProcess, error) {
				calls++
				return tt.process, tt.sourceErr
			})
			session := SessionSnapshot{
				ID:    sessionID,
				SID:   sid,
				State: tt.sessionState,
			}

			got, err := CaptureObservation(source, session, at)
			if !errors.Is(err, tt.sourceErr) {
				t.Errorf("CaptureObservation error = %v, want %v", err, tt.sourceErr)
			}
			want := tracking.Observation{
				At:             at,
				SessionID:      sessionID,
				SID:            sid,
				ExecutablePath: tt.wantPath,
				State:          tt.wantState,
			}
			if got != want {
				t.Errorf("observation = %+v, want %+v", got, want)
			}
			if calls != tt.wantCalls {
				t.Errorf("foreground queries = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

type foregroundSourceFunc func() (ForegroundProcess, error)

func (f foregroundSourceFunc) ForegroundProcess() (ForegroundProcess, error) {
	return f()
}
