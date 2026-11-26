package tracking

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
	"github.com/ebe542/go-screen-time/internal/storage/sqlite"
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

func TestPersistentTrackerRestoresPartialUsageAndActiveBreak(t *testing.T) {
	_, registry := newBreakTestTracker(t)
	path := filepath.Join(t.TempDir(), "usage.db")
	rule := policy.BreakRule{
		After:    45 * time.Minute,
		Duration: 10 * time.Minute,
	}

	open := func() (*Tracker, *sqlite.Ledger) {
		t.Helper()

		store, err := sqlite.Open(path, time.UTC)
		if err != nil {
			t.Fatalf("open SQLite ledger: %v", err)
		}
		tracker := NewPersistentTracker(registry, store, time.Hour)
		tracker.SetBreakRule(childA, rule)
		if err := tracker.RestoreBreakState(childA); err != nil {
			_ = store.Close()
			t.Fatalf("restore break state: %v", err)
		}
		return tracker, store
	}

	closeStore := func(store *sqlite.Ledger) {
		t.Helper()
		if err := store.Close(); err != nil {
			t.Fatalf("close SQLite ledger: %v", err)
		}
	}

	tracker, store := open()
	observeBreakTest(t, tracker, 0, gameA)
	observeBreakTest(t, tracker, 30, gameB)
	closeStore(store)

	tracker, store = open()
	if got := tracker.BreakState(childA); got.Used != 30*time.Minute || !got.StartedAt.IsZero() {
		_ = store.Close()
		t.Fatalf("restored partial break state = %+v, want 30m and no active break", got)
	}

	observeBreakTest(t, tracker, 33, gameB)
	observeBreakTest(t, tracker, 48, writer)
	wantStart := trackingStart.Add(48 * time.Minute)
	if got := tracker.BreakState(childA); got.Used != 45*time.Minute ||
		!got.StartedAt.Equal(wantStart) {
		t.Errorf("break state = %+v, want 45m and start at %v", got, wantStart)
	}
	closeStore(store)

	tracker, store = open()
	defer closeStore(store)

	restored := tracker.BreakState(childA)
	if !restored.StartedAt.Equal(wantStart) {
		t.Fatalf("restored break start = %v, want %v", restored.StartedAt, wantStart)
	}

	tests := []struct {
		at   time.Time
		want managedapps.AccessDecision
	}{
		{wantStart.Add(10*time.Minute - time.Second), managedapps.AccessBlocked},
		{wantStart.Add(10 * time.Minute), managedapps.AccessAllowed},
	}
	for _, tt := range tests {
		got, err := registry.EvaluateAccess(store, childA, gameA, tt.at, restored.StartedAt)
		if err != nil || got != tt.want {
			t.Errorf("access at %v = (%q, %v), want (%q, nil)",
				tt.at, got, err, tt.want)
		}
	}
}
