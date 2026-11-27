package control

import (
	"errors"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

// ObservationConnection identifies a helper authenticated by the service.
type ObservationConnection struct {
	ID        uint64
	SessionID string
	SID       string
}

// ObservationProcessor handles trusted observations in order.
type ObservationProcessor interface {
	Process(tracking.Observation) (managedapps.AccessDecision, error)
}

// ObservationRouter forwards observations from one active helper connection.
// The service calls its methods sequentially.
type ObservationRouter struct {
	processor ObservationProcessor
	active    ObservationConnection
}

// NewObservationRouter creates a router with no active connection.
func NewObservationRouter(processor ObservationProcessor) *ObservationRouter {
	return &ObservationRouter{processor: processor}
}

// Activate selects the connection for a session confirmed by the service.
func (r *ObservationRouter) Activate(connection ObservationConnection, at time.Time) error {
	if err := connection.validate(); err != nil {
		return err
	}
	if connection.ID == r.active.ID {
		if connection != r.active {
			return errors.New("active observation connection changed identity")
		}
		return nil
	}

	if r.active.ID != 0 {
		state := tracking.ObservationInactive
		if connection.SessionID == r.active.SessionID && connection.SID == r.active.SID {
			state = tracking.ObservationUnavailable
		}
		if err := r.forwardBoundary(at, state); err != nil {
			return err
		}
	}

	r.active = connection
	return nil
}

// Accept checks the connection and observation identity before forwarding.
func (r *ObservationRouter) Accept(
	connectionID uint64,
	observation tracking.Observation,
) (managedapps.AccessDecision, error) {
	if connectionID == 0 || connectionID != r.active.ID {
		return "", nil
	}
	if observation.SessionID != r.active.SessionID || observation.SID != r.active.SID {
		return "", errors.New("observation identity does not match the active connection")
	}

	return r.processor.Process(observation)
}

// Disconnect reports an observation gap for the active connection.
func (r *ObservationRouter) Disconnect(connectionID uint64, at time.Time) error {
	if connectionID == 0 || connectionID != r.active.ID {
		return nil
	}
	return r.clearActive(at, tracking.ObservationUnavailable)
}

func (r *ObservationRouter) clearActive(at time.Time, state tracking.ObservationState) error {
	if r.active.ID == 0 {
		return nil
	}
	if err := r.forwardBoundary(at, state); err != nil {
		return err
	}

	r.active = ObservationConnection{}
	return nil
}

func (c ObservationConnection) validate() error {
	if c.ID == 0 || c.SessionID == "" || c.SID == "" {
		return errors.New("observation connection requires an ID, session ID, and SID")
	}
	return nil
}

func (r *ObservationRouter) forwardBoundary(at time.Time, state tracking.ObservationState) error {
	_, err := r.processor.Process(tracking.Observation{
		At:        at,
		SessionID: r.active.SessionID,
		SID:       r.active.SID,
		State:     state,
	})
	return err
}
