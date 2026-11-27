// Package control coordinates usage accounting and app access decisions.
package control

import (
	"fmt"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

// Store provides the operations needed to record usage and evaluate access.
type Store interface {
	tracking.BreakStateStore
	managedapps.AccessStore
}

// Controller processes an ordered stream of session observations.
type Controller struct {
	registry *managedapps.Registry
	store    Store
	tracker  *tracking.Tracker
	lastAt   time.Time
}

// NewController creates a controller and restores configured break states.
func NewController(
	registry *managedapps.Registry,
	store Store,
	maxGap time.Duration,
) (*Controller, error) {
	tracker := tracking.NewPersistentTracker(registry, store, maxGap)
	for childID, rule := range registry.BreakRules() {
		tracker.SetBreakRule(childID, rule)
		if err := tracker.RestoreBreakState(childID); err != nil {
			return nil, fmt.Errorf("restore break state for child %q: %w", childID, err)
		}
	}

	return &Controller{registry: registry, store: store, tracker: tracker}, nil
}

// Process records the preceding interval and evaluates the current app.
// Observations without a foreground app produce no access decision.
func (c *Controller) Process(
	observation tracking.Observation,
) (managedapps.AccessDecision, error) {
	if observation.At.Before(c.lastAt) {
		return "", nil
	}
	if err := c.tracker.Observe(observation); err != nil {
		return "", err
	}
	c.lastAt = observation.At

	if observation.State != tracking.ObservationForeground {
		return "", nil
	}

	state := c.tracker.BreakState(observation.SID)
	return c.registry.EvaluateAccess(
		c.store,
		observation.SID,
		observation.ExecutablePath,
		observation.At,
		state.StartedAt,
	)
}
