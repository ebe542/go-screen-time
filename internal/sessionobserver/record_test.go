package sessionobserver

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
	"github.com/ebe542/go-screen-time/internal/storage/sqlite"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestRecordMonitorUsagePersistsOnlyConfirmedForeground(t *testing.T) {
	const sessionID = "12"
	const childID = "S-1-5-21-100-200-300-1001"
	const game = `C:\Games\Example\game.exe`
	const writer = `C:\School\writer.exe`

	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	dbPath := filepath.Join(t.TempDir(), "usage.db")
	readErr := errors.New("foreground process unavailable")

	registry, err := managedapps.NewRegistry([]managedapps.ChildProfile{{
		SID: childID,
		Apps: []managedapps.App{
			{
				ID:              "game",
				ExecutablePaths: []string{game},
				Policy: &policy.AppAccessPolicy{
					SharedBudget: policy.DailyBudget{WeekdayLimit: time.Hour},
				},
			},
			{ID: "writer", ExecutablePaths: []string{writer}},
		},
	}})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	type sample struct {
		second int
		path   string
		err    error
	}

	run := func(samples []sample, wantReadErrors int, wantUsage time.Duration) {
		t.Helper()

		store, err := sqlite.Open(dbPath, time.UTC)
		if err != nil {
			t.Fatalf("open SQLite ledger: %v", err)
		}
		defer store.Close()

		session := NewSessionTracker(sessionID, childID)
		session.Apply(SessionEvent{SessionID: sessionID, Kind: SessionActivated})
		session.Apply(SessionEvent{SessionID: sessionID, Kind: SessionUnlocked})

		events := make(chan MonitorEvent, len(samples))
		for _, s := range samples {
			events <- MonitorEvent{
				At:   start.Add(time.Duration(s.second) * time.Second),
				Kind: MonitorHeartbeat,
			}
		}
		close(events)

		next := 0
		source := foregroundSourceFunc(func() (ForegroundProcess, error) {
			s := samples[next]
			next++
			if s.err != nil {
				return ForegroundProcess{}, s.err
			}
			return ForegroundProcess{
				SessionID:      sessionID,
				SID:            childID,
				ExecutablePath: s.path,
			}, nil
		})

		tracker := tracking.NewTracker(registry, store, 15*time.Second)
		reported := 0
		err = RecordMonitorUsage(
			context.Background(), NewMonitor(session, source), events, tracker,
			func(err error) {
				reported++
				if !errors.Is(err, readErr) {
					t.Errorf("reported error = %v, want %v", err, readErr)
				}
			},
		)
		if err != nil {
			t.Fatalf("record monitor usage: %v", err)
		}
		if reported != wantReadErrors {
			t.Errorf("reported %d read errors, want %d", reported, wantReadErrors)
		}

		got, err := store.BudgetTotals(childID, "game", start)
		if err != nil {
			t.Fatalf("read game usage: %v", err)
		}
		if got.SharedRegular != wantUsage || got.AppRegular != wantUsage || got.AppBonus != 0 {
			t.Errorf("game budget usage = %+v, want %s shared and app regular", got, wantUsage)
		}

		writerUsage, err := store.BudgetTotals(childID, "writer", start)
		if err != nil {
			t.Fatalf("read writer usage: %v", err)
		}
		if writerUsage.AppRegular != 0 || writerUsage.AppBonus != 0 {
			t.Errorf("unrestricted writer consumed media budget: %+v", writerUsage)
		}
	}

	run([]sample{
		{second: 0, path: game},
		{second: 5, path: writer},
		{second: 10, path: game},
		{second: 15, err: readErr},
		{second: 20, path: game},
		{second: 25, path: game},
	}, 1, 10*time.Second)

	// Reopen the database and start a new tracker three seconds later.
	// Those three seconds must never be charged to either app.
	run([]sample{
		{second: 28, path: game},
		{second: 33, path: writer},
	}, 0, 15*time.Second)
}
