// Package tracking turns ordered foreground observations into usage bookings.
package tracking

import (
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
)

// ObservationState describes what is known about the interactive session.
type ObservationState uint8

const (
	ObservationUnavailable ObservationState = iota
	ObservationForeground
	ObservationInactive
)

// Observation describes the active session and its foreground executable.
type Observation struct {
	At             time.Time
	SessionID      string
	SID            string
	ExecutablePath string
	State          ObservationState
}

// UsageRecorder stores a foreground interval using the assigned access policy.
type UsageRecorder interface {
	RecordAppUsageWithGrants(
		childID, appID string,
		from, to time.Time,
		access policy.AppAccessPolicy,
	) error
}

// Tracker records usage from a single ordered stream of observations.
type Tracker struct {
	registry *managedapps.Registry
	store    UsageRecorder
	maxGap   time.Duration

	lastAt time.Time
	active managedapps.Assignment

	lastSID   string
	idleSince time.Time
	breaks    map[string]*childBreak

	breakStore BreakStateStore
}

// NewTracker creates a tracker with the given maximum observation gap.
func NewTracker(
	registry *managedapps.Registry,
	store UsageRecorder,
	maxGap time.Duration,
) *Tracker {
	return &Tracker{registry: registry, store: store, maxGap: maxGap}
}

// Observe books the previous foreground interval and records the new state.
func (t *Tracker) Observe(observation Observation) error {
	if observation.At.Before(t.lastAt) {
		return nil
	}

	elapsed := observation.At.Sub(t.lastAt)
	knownBoundary := observation.State == ObservationForeground ||
		observation.State == ObservationInactive
	continuous := knownBoundary && t.lastSID != "" && elapsed <= t.maxGap

	if continuous && elapsed > 0 {
		if t.active.Policy != nil {
			if err := t.recordCurrentUsage(observation.At); err != nil {
				return err
			}
		} else if err := t.recordBreakIdle(observation.At); err != nil {
			return err
		}
	}

	var next managedapps.Assignment
	if observation.State == ObservationForeground {
		next, _ = t.registry.Match(observation.SID, observation.ExecutablePath)
	}

	switch {
	case !knownBoundary || next.Policy != nil:
		t.idleSince = time.Time{}
	case !continuous || t.lastSID != observation.SID || t.idleSince.IsZero():
		t.idleSince = observation.At
	}

	t.lastAt = observation.At
	t.lastSID = ""
	if knownBoundary {
		t.lastSID = observation.SID
	}
	t.active = next
	return nil
}
