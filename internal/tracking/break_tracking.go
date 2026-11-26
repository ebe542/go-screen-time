package tracking

import (
	"time"

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
