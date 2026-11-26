// Package sessionobserver converts session and process snapshots into tracking observations.
package sessionobserver

import (
	"time"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

// SessionState describes whether a session can contribute foreground usage.
type SessionState uint8

const (
	SessionUnknown SessionState = iota
	SessionActive
	SessionInactive
)

// SessionSnapshot identifies the session and its signed-in account.
type SessionSnapshot struct {
	ID    string
	SID   string
	State SessionState
}

// ForegroundProcess identifies the process that owns the foreground window.
type ForegroundProcess struct {
	SessionID      string
	SID            string
	ExecutablePath string
}

// ForegroundSource reads the current foreground process.
// A zero process and nil error indicate that no foreground window was found.
type ForegroundSource interface {
	ForegroundProcess() (ForegroundProcess, error)
}

// CaptureObservation converts a session snapshot into a tracking observation.
// A source error is returned alongside an unavailable observation.
func CaptureObservation(
	source ForegroundSource,
	session SessionSnapshot,
	at time.Time,
) (tracking.Observation, error) {
	observation := tracking.Observation{
		At:        at,
		SessionID: session.ID,
		SID:       session.SID,
		State:     tracking.ObservationUnavailable,
	}

	switch session.State {
	case SessionInactive:
		observation.State = tracking.ObservationInactive
		return observation, nil
	case SessionActive:
		if session.ID == "" || session.SID == "" {
			return observation, nil
		}
	default:
		return observation, nil
	}

	process, err := source.ForegroundProcess()
	if err != nil {
		return observation, err
	}
	if process.SessionID != session.ID ||
		process.SID != session.SID ||
		process.ExecutablePath == "" {
		return observation, nil
	}

	observation.State = tracking.ObservationForeground
	observation.ExecutablePath = process.ExecutablePath
	return observation, nil
}
