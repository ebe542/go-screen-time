package policy

import (
	"time"
)

// BreakRule defines when accumulated usage triggers a required break.
type BreakRule struct {
	After    time.Duration
	Duration time.Duration
}

// RequiresBreak reports whether usage has reached a positive break threshold.
func (r BreakRule) RequiresBreak(used time.Duration) bool {
	return r.After > 0 && used >= r.After
}

// IsActive reports whether at falls within the break that began at startedAt.
func (r BreakRule) IsActive(at, startedAt time.Time) bool {
	return !at.Before(startedAt) && at.Before(startedAt.Add(r.Duration))
}

// AfterIdle resets the usage counter after a full interruption or break.
// A zero idleSince means no voluntary interruption is being tracked.
func (r BreakRule) AfterIdle(
	state BreakState,
	idleSince, at time.Time,
) BreakState {
	if r.Duration <= 0 {
		return state
	}

	resetFrom := idleSince
	if !state.StartedAt.IsZero() {
		resetFrom = state.StartedAt
	}

	if !resetFrom.IsZero() && !at.Before(resetFrom.Add(r.Duration)) {
		return BreakState{}
	}
	return state
}

// BreakState tracks one child's accumulated usage and break start.
type BreakState struct {
	Used      time.Duration
	StartedAt time.Time
}

// AddUsage records a managed usage interval ending at endedAt.
// Once a break starts, its start time is preserved until the state is reset.
func (r BreakRule) AddUsage(
	state BreakState,
	endedAt time.Time,
	used time.Duration,
) BreakState {
	if !state.StartedAt.IsZero() {
		return state
	}

	state.Used += used
	if r.RequiresBreak(state.Used) {
		overshoot := state.Used - r.After
		state.StartedAt = endedAt.Add(-overshoot)
		state.Used = r.After
	}
	return state
}
