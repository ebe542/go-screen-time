package control

import (
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

// ObservationIntake routes authenticated helpers based on the service's
// active console session. The service calls its methods sequentially.
type ObservationIntake struct {
	router          *ObservationRouter
	activeSessionID string
	bySession       map[string]ObservationConnection
}

// NewObservationIntake creates an intake with no active console session.
func NewObservationIntake(processor ObservationProcessor) *ObservationIntake {
	return &ObservationIntake{
		router:    NewObservationRouter(processor),
		bySession: make(map[string]ObservationConnection),
	}
}

// SetActiveSession selects the console session confirmed by the service.
// An empty session ID indicates that the active session is unavailable.
func (i *ObservationIntake) SetActiveSession(sessionID string, at time.Time) error {
	if sessionID == i.activeSessionID {
		return nil
	}

	if connection, found := i.bySession[sessionID]; found {
		if err := i.router.Activate(connection, at); err != nil {
			return err
		}
	} else {
		state := tracking.ObservationInactive
		if sessionID == "" {
			state = tracking.ObservationUnavailable
		}
		if err := i.router.clearActive(at, state); err != nil {
			return err
		}
	}

	i.activeSessionID = sessionID
	return nil
}

// Connect registers a helper whose identity was verified by the transport.
func (i *ObservationIntake) Connect(connection ObservationConnection, at time.Time) error {
	if err := connection.validate(); err != nil {
		return err
	}

	if connection.SessionID == i.activeSessionID {
		if err := i.router.Activate(connection, at); err != nil {
			return err
		}
	}

	i.bySession[connection.SessionID] = connection
	return nil
}

// Accept forwards an observation only from the currently active connection.
func (i *ObservationIntake) Accept(
	connectionID uint64,
	observation tracking.Observation,
) (managedapps.AccessDecision, error) {
	return i.router.Accept(connectionID, observation)
}

// Disconnect removes a helper and reports a gap if it was active.
func (i *ObservationIntake) Disconnect(connectionID uint64, at time.Time) error {
	if err := i.router.Disconnect(connectionID, at); err != nil {
		return err
	}

	for sessionID, connection := range i.bySession {
		if connection.ID == connectionID {
			delete(i.bySession, sessionID)
			break
		}
	}
	return nil
}
