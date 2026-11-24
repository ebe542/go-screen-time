package policy

import (
	"testing"
	"time"
)

func TestBreakRuleRequiresBreak(t *testing.T) {
	rule := BreakRule{After: 45 * time.Minute}

	tests := []struct {
		name string
		used time.Duration
		want bool
	}{
		{"before threshold", 45*time.Minute - time.Second, false},
		{"at threshold", 45 * time.Minute, true},
		{"after threshold", 45*time.Minute + time.Second, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rule.RequiresBreak(tt.used); got != tt.want {
				t.Errorf("RequiresBreak(%s) = %t, want %t", tt.used, got, tt.want)
			}
		})
	}
}

func TestBreakRuleIsActive(t *testing.T) {
	rule := BreakRule{
		After:    45 * time.Minute,
		Duration: 10 * time.Minute,
	}
	startedAt := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		offset time.Duration
		want   bool
	}{
		{"before break starts", -time.Second, false},
		{"at break start", 0, true},
		{"just before break ends", 10*time.Minute - time.Second, true},
		{"at break end", 10 * time.Minute, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := startedAt.Add(tt.offset)
			if got := rule.IsActive(at, startedAt); got != tt.want {
				t.Errorf("IsActive at offset %s = %t, want %t", tt.offset, got, tt.want)
			}
		})
	}
}

func TestBreakRuleWithoutConfiguration(t *testing.T) {
	var rule BreakRule

	for _, used := range []time.Duration{0, time.Hour} {
		if rule.RequiresBreak(used) {
			t.Errorf("RequiresBreak(%s) = true for an unconfigured rule", used)
		}
	}
}

func TestBreakRuleAccumulatesUsageAcrossApps(t *testing.T) {
	rule := BreakRule{
		After:    45 * time.Minute,
		Duration: 10 * time.Minute,
	}
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

	var state BreakState

	firstEnd := start.Add(20 * time.Minute)
	state = rule.AddUsage(state, firstEnd, 20*time.Minute)
	if state.Used != 20*time.Minute || !state.StartedAt.IsZero() {
		t.Fatalf("after first app: used = %s, break started at %v",
			state.Used, state.StartedAt)
	}

	secondEnd := firstEnd.Add(25 * time.Minute)
	state = rule.AddUsage(state, secondEnd, 25*time.Minute)
	if !state.StartedAt.Equal(secondEnd) {
		t.Errorf("break started at %v, want %v", state.StartedAt, secondEnd)
	}
}

func TestBreakRuleStartsAtThresholdWithinInterval(t *testing.T) {
	rule := BreakRule{
		After:    45 * time.Minute,
		Duration: 10 * time.Minute,
	}
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

	state := rule.AddUsage(BreakState{}, start.Add(44*time.Minute), 44*time.Minute)
	state = rule.AddUsage(state, start.Add(46*time.Minute), 2*time.Minute)

	wantStartedAt := start.Add(45 * time.Minute)
	if !state.StartedAt.Equal(wantStartedAt) {
		t.Errorf("break started at %v, want %v", state.StartedAt, wantStartedAt)
	}
	if state.Used != 45*time.Minute {
		t.Errorf("accumulated usage = %s, want 45m", state.Used)
	}
}

func TestBreakRuleAfterIdle(t *testing.T) {
	rule := BreakRule{
		After:    45 * time.Minute,
		Duration: 10 * time.Minute,
	}
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	partial := BreakState{Used: 20 * time.Minute}
	active := BreakState{Used: 45 * time.Minute, StartedAt: start}

	tests := []struct {
		name      string
		state     BreakState
		idleSince time.Time
		at        time.Time
		want      BreakState
	}{
		{"short interruption", partial, start, start.Add(9 * time.Minute), partial},
		{"full interruption", partial, start, start.Add(10 * time.Minute), BreakState{}},
		{"required break still active", active, start, start.Add(9 * time.Minute), active},
		{"required break ends", active, start.Add(time.Minute), start.Add(10 * time.Minute), BreakState{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rule.AfterIdle(tt.state, tt.idleSince, tt.at)
			if got.Used != tt.want.Used || !got.StartedAt.Equal(tt.want.StartedAt) {
				t.Errorf("AfterIdle = %+v, want %+v", got, tt.want)
			}
		})
	}
}
