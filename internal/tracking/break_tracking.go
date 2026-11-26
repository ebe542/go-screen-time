package tracking

import (
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
)

type childBreak struct {
	rule  policy.BreakRule
	state policy.BreakState
}

// SetBreakRule sets the break rule for one child.
func (t *Tracker) SetBreakRule(childID string, rule policy.BreakRule) {
	if rule.After <= 0 || rule.Duration <= 0 {
		delete(t.breaks, childID)
		return
	}
	if t.breaks == nil {
		t.breaks = make(map[string]*childBreak)
	}
	current := t.breaks[childID]
	if current != nil && current.rule == rule {
		return
	}
	t.breaks[childID] = &childBreak{rule: rule}
}

// BreakState returns the latest break state for the child.
func (t *Tracker) BreakState(childID string) policy.BreakState {
	if current := t.breaks[childID]; current != nil {
		return current.state
	}
	return policy.BreakState{}
}

func (b *childBreak) addUsage(from, to time.Time) {
	if !b.state.StartedAt.IsZero() {
		resumeAt := b.state.StartedAt.Add(b.rule.Duration)
		b.state = b.rule.AfterIdle(b.state, time.Time{}, to)

		if resumeAt.After(from) {
			from = resumeAt
		}
	}
	if from.Before(to) {
		b.state = b.rule.AddUsage(b.state, to, to.Sub(from))
	}
}

// BreakStateStore persists break progress alongside usage.
type BreakStateStore interface {
	UsageRecorder

	LoadBreakState(childID string) (policy.BreakState, error)
	SaveBreakState(childID string, state policy.BreakState) error
	RecordAppUsageAndBreakState(
		childID, appID string,
		start, end time.Time,
		access policy.AppAccessPolicy,
		state policy.BreakState,
	) error
}

// NewPersistentTracker creates a tracker that can persist break state.
func NewPersistentTracker(
	registry *managedapps.Registry,
	store BreakStateStore,
	maxGap time.Duration,
) *Tracker {
	tracker := NewTracker(registry, store, maxGap)
	tracker.breakStore = store
	return tracker
}

// RestoreBreakState loads the state after the child's rule has been configured.
func (t *Tracker) RestoreBreakState(childID string) error {
	current := t.breaks[childID]
	if current == nil || t.breakStore == nil {
		return nil
	}
	state, err := t.breakStore.LoadBreakState(childID)
	if err != nil {
		return err
	}
	current.state = state
	return nil
}

func (t *Tracker) recordCurrentUsage(to time.Time) error {
	active := t.active
	current := t.breaks[active.ChildID]
	if current == nil {
		return t.store.RecordAppUsageWithGrants(
			active.ChildID, active.AppID, t.lastAt, to, *active.Policy,
		)
	}

	next := *current
	next.addUsage(t.lastAt, to)

	var err error
	if t.breakStore != nil {
		err = t.breakStore.RecordAppUsageAndBreakState(
			active.ChildID, active.AppID, t.lastAt, to, *active.Policy, next.state,
		)
	} else {
		err = t.store.RecordAppUsageWithGrants(
			active.ChildID, active.AppID, t.lastAt, to, *active.Policy,
		)
	}
	if err != nil {
		return err
	}

	current.state = next.state
	return nil
}

func (t *Tracker) recordBreakIdle(at time.Time) error {
	current := t.breaks[t.lastSID]
	if current == nil {
		return nil
	}

	next := current.rule.AfterIdle(current.state, t.idleSince, at)
	if next == current.state {
		return nil
	}
	if t.breakStore != nil {
		if err := t.breakStore.SaveBreakState(t.lastSID, next); err != nil {
			return err
		}
	}

	current.state = next
	return nil
}
