package control

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
	"github.com/ebe542/go-screen-time/internal/storage/sqlite"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

const (
	childSID = "S-1-5-21-100-200-300-1001"
	gameA    = `C:\Games\A\game.exe`
	gameB    = `C:\Games\B\game.exe`
	writer   = `C:\School\writer.exe`
)

var start = time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

func TestControllerBooksBeforeDecidingAccessAndRestoresBreak(t *testing.T) {
	registry, rules := testControllerConfig(t)
	path := filepath.Join(t.TempDir(), "usage.db")

	store := openControllerStore(t, path)
	defer func() {
		if store != nil {
			_ = store.Close()
		}
	}()

	controller, err := NewController(registry, store, time.Hour, rules)
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}

	assertControllerDecision(t, controller, 0, gameA, managedapps.AccessAllowed)
	assertControllerDecision(t, controller, 20, gameA, managedapps.AccessAllowed)
	assertControllerDecision(t, controller, 45, gameB, managedapps.AccessBlocked)
	assertControllerDecision(t, controller, 45, writer, managedapps.AccessAllowed)

	if err := store.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}
	store = nil

	store = openControllerStore(t, path)
	controller, err = NewController(registry, store, time.Hour, rules)
	if err != nil {
		t.Fatalf("restart controller: %v", err)
	}

	// The first decision after restart already uses the saved break.
	assertControllerDecision(t, controller, 50, gameB, managedapps.AccessBlocked)
	assertControllerDecision(t, controller, 55, gameB, managedapps.AccessAllowed)
}

func TestControllerSkipsAccessCheckWhenUsageBookingFails(t *testing.T) {
	registry, rules := testControllerConfig(t)
	ledger := openControllerStore(t, filepath.Join(t.TempDir(), "usage.db"))
	defer ledger.Close()

	writeErr := errors.New("usage booking failed")
	store := &failingControllerStore{Ledger: ledger, writeErr: writeErr}
	controller, err := NewController(registry, store, time.Hour, rules)
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}

	assertControllerDecision(t, controller, 0, gameA, managedapps.AccessAllowed)
	before := store.accessChecks
	observation := foregroundObservation(45, gameB)

	got, err := controller.Process(observation)
	if !errors.Is(err, writeErr) || got != "" {
		t.Errorf("Process = (%q, %v), want no decision and %v", got, err, writeErr)
	}
	if store.accessChecks != before {
		t.Errorf("access checks after failed booking = %d, want %d",
			store.accessChecks, before)
	}

	store.writeErr = nil
	got, err = controller.Process(observation)
	if err != nil || got != managedapps.AccessBlocked {
		t.Errorf("retry = (%q, %v), want blocked", got, err)
	}
}

func testControllerConfig(t *testing.T) (*managedapps.Registry, map[string]policy.BreakRule) {
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
	registry, err := managedapps.NewRegistry([]managedapps.ChildProfile{{
		SID: childSID,
		Apps: []managedapps.App{
			{ID: "game-a", ExecutablePaths: []string{gameA}, Policy: timed},
			{ID: "game-b", ExecutablePaths: []string{gameB}, Policy: timed},
			{ID: "writer", ExecutablePaths: []string{writer}},
		},
	}})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}
	return registry, map[string]policy.BreakRule{childSID: rule}
}

func openControllerStore(t *testing.T, path string) *sqlite.Ledger {
	t.Helper()

	store, err := sqlite.Open(path, time.UTC)
	if err != nil {
		t.Fatalf("open SQLite ledger: %v", err)
	}
	return store
}

func foregroundObservation(minute int, path string) tracking.Observation {
	return tracking.Observation{
		At:             start.Add(time.Duration(minute) * time.Minute),
		SessionID:      "12",
		SID:            childSID,
		ExecutablePath: path,
		State:          tracking.ObservationForeground,
	}
}

func assertControllerDecision(
	t *testing.T,
	controller *Controller,
	minute int,
	path string,
	want managedapps.AccessDecision,
) {
	t.Helper()

	got, err := controller.Process(foregroundObservation(minute, path))
	if err != nil || got != want {
		t.Fatalf("Process at +%dm = (%q, %v), want (%q, nil)", minute, got, err, want)
	}
}

type failingControllerStore struct {
	*sqlite.Ledger
	writeErr     error
	accessChecks int
}

func (s *failingControllerStore) RecordAppUsageAndBreakState(
	childID, appID string,
	from, to time.Time,
	access policy.AppAccessPolicy,
	state policy.BreakState,
) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	return s.Ledger.RecordAppUsageAndBreakState(childID, appID, from, to, access, state)
}

func (s *failingControllerStore) AllowsAppWithGrants(
	childID, appID string,
	at time.Time,
	access policy.AppAccessPolicy,
	breakStartedAt time.Time,
) (bool, error) {
	s.accessChecks++
	return s.Ledger.AllowsAppWithGrants(childID, appID, at, access, breakStartedAt)
}
