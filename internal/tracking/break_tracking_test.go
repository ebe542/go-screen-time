package tracking

import (
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
)

func TestTrackerStartsSharedBreakAcrossApps(t *testing.T) {
	tracker, registry := newBreakTestTracker(t)

	observeBreakTest(t, tracker, 0, gameA)
	observeBreakTest(t, tracker, 20, gameB)

	if got := tracker.BreakState(childA).Used; got != 20*time.Minute {
		t.Errorf("usage before app switch = %s, want 20m", got)
	}

	observeBreakTest(t, tracker, 45, writer)
	state := tracker.BreakState(childA)
	startedAt := trackingStart.Add(45 * time.Minute)
	if !state.StartedAt.Equal(startedAt) {
		t.Fatalf("break started at %v, want %v", state.StartedAt, startedAt)
	}
	if got := tracker.BreakState(childB); got != (policy.BreakState{}) {
		t.Errorf("other child's break state = %+v, want zero state", got)
	}

	tests := []struct {
		name string
		path string
		at   time.Time
		want managedapps.AccessDecision
	}{
		{"first game during break", gameA, startedAt, managedapps.AccessBlocked},
		{"second game during break", gameB, startedAt, managedapps.AccessBlocked},
		{"school app during break", writer, startedAt, managedapps.AccessAllowed},
		{"game at break end", gameA, startedAt.Add(10 * time.Minute), managedapps.AccessAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := registry.EvaluateAccess(
				breakAccessStore{}, childA, tt.path, tt.at, state.StartedAt,
			)
			if err != nil || got != tt.want {
				t.Errorf("EvaluateAccess = (%q, %v), want (%q, nil)", got, err, tt.want)
			}
		})
	}
}

func TestTrackerResetsBreakCounterAfterConfirmedInterruption(t *testing.T) {
	tracker, _ := newBreakTestTracker(t)

	observeBreakTest(t, tracker, 0, gameA)
	observeBreakTest(t, tracker, 20, writer)
	observeBreakTest(t, tracker, 25, gameA)

	if got := tracker.BreakState(childA).Used; got != 20*time.Minute {
		t.Errorf("usage after short interruption = %s, want 20m", got)
	}

	observeBreakTest(t, tracker, 30, writer)
	observeBreakTest(t, tracker, 40, gameB)

	if got := tracker.BreakState(childA); got != (policy.BreakState{}) {
		t.Errorf("state after full interruption = %+v, want zero state", got)
	}
}

func TestTrackerDoesNotTreatUnknownTimeAsUseOrRest(t *testing.T) {
	tracker, registry := newBreakTestTracker(t)

	observeBreakTest(t, tracker, 0, gameA)
	observeBreakTest(t, tracker, 20, gameA)
	observeUnavailableForBreakTest(t, tracker, 25)
	observeBreakTest(t, tracker, 35, writer)
	observeBreakTest(t, tracker, 40, gameB)

	if got := tracker.BreakState(childA).Used; got != 20*time.Minute {
		t.Fatalf("usage after observation loss = %s, want 20m", got)
	}

	observeBreakTest(t, tracker, 65, gameB)
	startedAt := tracker.BreakState(childA).StartedAt
	if want := trackingStart.Add(65 * time.Minute); !startedAt.Equal(want) {
		t.Fatalf("break started at %v, want %v", startedAt, want)
	}

	observeUnavailableForBreakTest(t, tracker, 70)
	got, err := registry.EvaluateAccess(
		breakAccessStore{}, childA, gameB,
		startedAt.Add(10*time.Minute), startedAt,
	)
	if err != nil || got != managedapps.AccessAllowed {
		t.Errorf("access at break end = (%q, %v), want allowed", got, err)
	}
}

func newBreakTestTracker(t *testing.T) (*Tracker, *managedapps.Registry) {
	t.Helper()

	endOfDay, err := policy.NewTimeOfDay(24, 0)
	if err != nil {
		t.Fatalf("create end of day: %v", err)
	}
	window, err := policy.NewUsageWindow(policy.TimeOfDay{}, endOfDay)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	rule := policy.BreakRule{After: 45 * time.Minute, Duration: 10 * time.Minute}
	timed := &policy.AppAccessPolicy{
		Schedule:     policy.UsageSchedule{WeekdayWindows: []policy.UsageWindow{window}},
		SharedBudget: policy.DailyBudget{WeekdayLimit: 2 * time.Hour},
		BreakRule:    rule,
	}
	registry, err := managedapps.NewRegistry([]managedapps.ChildProfile{
		{
			SID: childA,
			Apps: []managedapps.App{
				{ID: "game-a", ExecutablePaths: []string{gameA}, Policy: timed},
				{ID: "game-b", ExecutablePaths: []string{gameB}, Policy: timed},
				{ID: "writer", ExecutablePaths: []string{writer}},
			},
		},
		{SID: childB},
	})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	tracker := NewTracker(registry, &recordingStore{}, time.Hour)
	tracker.SetBreakRule(childA, rule)
	tracker.SetBreakRule(childB, rule)
	return tracker, registry
}

func observeBreakTest(t *testing.T, tracker *Tracker, minute int, path string) {
	t.Helper()
	if err := tracker.Observe(foreground(minute*60, "session-a", childA, path)); err != nil {
		t.Fatalf("observe foreground at minute %d: %v", minute, err)
	}
}

func observeUnavailableForBreakTest(t *testing.T, tracker *Tracker, minute int) {
	t.Helper()
	if err := tracker.Observe(Observation{
		At:        trackingStart.Add(time.Duration(minute) * time.Minute),
		SessionID: "session-a",
		SID:       childA,
		State:     ObservationUnavailable,
	}); err != nil {
		t.Fatalf("observe unavailable at minute %d: %v", minute, err)
	}
}

type breakAccessStore struct{}

func (breakAccessStore) AllowsAppWithGrants(
	_, _ string,
	at time.Time,
	access policy.AppAccessPolicy,
	breakStartedAt time.Time,
) (bool, error) {
	return access.Allows(at, 0, 0, breakStartedAt), nil
}
