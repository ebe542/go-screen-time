package control

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
	"github.com/ebe542/go-screen-time/internal/storage/sqlite"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestObservationIntakeBooksOnlyTheActiveSession(t *testing.T) {
	intake, store := newIntakeTest(t)
	first := ObservationConnection{ID: 1, SessionID: "12", SID: childSID}
	second := ObservationConnection{
		ID: 2, SessionID: "13", SID: "S-1-5-21-100-200-300-1002",
	}

	if err := intake.SetActiveSession(first.SessionID, start); err != nil {
		t.Fatalf("select first session: %v", err)
	}
	for _, connection := range []ObservationConnection{first, second} {
		if err := intake.Connect(connection, start); err != nil {
			t.Fatalf("connect helper: %v", err)
		}
	}

	assertIntakeDecision(t, intake, first, 0, managedapps.AccessAllowed)
	assertIntakeDecision(t, intake, second, 3, "")

	if err := intake.SetActiveSession(second.SessionID, start.Add(5*time.Second)); err != nil {
		t.Fatalf("switch to second session: %v", err)
	}
	assertIntakeDecision(t, intake, first, 6, "")
	assertIntakeDecision(t, intake, second, 6, managedapps.AccessAllowed)
	assertIntakeDecision(t, intake, second, 11, managedapps.AccessAllowed)

	assertIntakeUsage(t, store, first.SID, 5*time.Second)
	assertIntakeUsage(t, store, second.SID, 5*time.Second)

	// Windows can switch to a session whose helper has not connected yet.
	if err := intake.SetActiveSession("14", start.Add(13*time.Second)); err != nil {
		t.Fatalf("switch to session without a helper: %v", err)
	}
	assertIntakeDecision(t, intake, second, 14, "")
	assertIntakeUsage(t, store, second.SID, 7*time.Second)
}

func TestObservationIntakeLeavesReconnectGapUnbooked(t *testing.T) {
	intake, store := newIntakeTest(t)
	first := ObservationConnection{ID: 1, SessionID: "12", SID: childSID}

	if err := intake.SetActiveSession(first.SessionID, start); err != nil {
		t.Fatalf("select active session: %v", err)
	}
	if err := intake.Connect(first, start); err != nil {
		t.Fatalf("connect first helper: %v", err)
	}

	assertIntakeDecision(t, intake, first, 0, managedapps.AccessAllowed)
	assertIntakeDecision(t, intake, first, 5, managedapps.AccessAllowed)

	if err := intake.Disconnect(first.ID, start.Add(8*time.Second)); err != nil {
		t.Fatalf("disconnect first helper: %v", err)
	}

	second := first
	second.ID = 2
	if err := intake.Connect(second, start.Add(10*time.Second)); err != nil {
		t.Fatalf("reconnect helper: %v", err)
	}

	assertIntakeDecision(t, intake, second, 10, managedapps.AccessAllowed)
	assertIntakeDecision(t, intake, first, 11, "")
	assertIntakeDecision(t, intake, second, 15, managedapps.AccessAllowed)

	// Only 0–5 and 10–15 are confirmed usage.
	assertIntakeUsage(t, store, childSID, 10*time.Second)
}

func newIntakeTest(t *testing.T) (*ObservationIntake, *sqlite.Ledger) {
	t.Helper()

	endOfDay, err := policy.NewTimeOfDay(24, 0)
	if err != nil {
		t.Fatalf("create end of day: %v", err)
	}
	window, err := policy.NewUsageWindow(policy.TimeOfDay{}, endOfDay)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	app := managedapps.App{
		ID:              "game",
		ExecutablePaths: []string{gameA},
		Policy: &policy.AppAccessPolicy{
			Schedule: policy.UsageSchedule{
				WeekdayWindows: []policy.UsageWindow{window},
			},
			SharedBudget: policy.DailyBudget{WeekdayLimit: time.Hour},
		},
	}

	registry, err := managedapps.NewRegistry([]managedapps.ChildProfile{
		{SID: childSID, Apps: []managedapps.App{app}},
		{SID: "S-1-5-21-100-200-300-1002", Apps: []managedapps.App{app}},
	})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	store := openControllerStore(t, filepath.Join(t.TempDir(), "usage.db"))
	t.Cleanup(func() { _ = store.Close() })

	controller, err := NewController(registry, store, 15*time.Second)
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}
	return NewObservationIntake(controller), store
}

func assertIntakeDecision(
	t *testing.T,
	intake *ObservationIntake,
	connection ObservationConnection,
	second int,
	want managedapps.AccessDecision,
) {
	t.Helper()

	got, err := intake.Accept(connection.ID, tracking.Observation{
		At:             start.Add(time.Duration(second) * time.Second),
		SessionID:      connection.SessionID,
		SID:            connection.SID,
		ExecutablePath: gameA,
		State:          tracking.ObservationForeground,
	})
	if err != nil || got != want {
		t.Errorf("decision for connection %d at +%ds = (%q, %v), want (%q, nil)",
			connection.ID, second, got, err, want)
	}
}

func assertIntakeUsage(t *testing.T, store *sqlite.Ledger, sid string, want time.Duration) {
	t.Helper()

	got, err := store.BudgetTotals(sid, "game", start)
	if err != nil {
		t.Fatalf("read usage for %s: %v", sid, err)
	}
	if got.SharedRegular != want || got.AppRegular != want {
		t.Errorf("usage for %s = %+v, want %s shared and app regular", sid, got, want)
	}
}
