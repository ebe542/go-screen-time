package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestUsageDiagnosticKeepsTotalsAcrossRestart(t *testing.T) {
	const game = `C:\Games\Example\game.exe`
	const writer = `C:\School\writer.exe`
	const sid = "S-1-5-21-100-200-300-1001"

	dbPath := filepath.Join(t.TempDir(), "foreground-test.db")
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

	type step struct {
		second int
		path   string
		state  tracking.ObservationState
		want   time.Duration
	}

	run := func(steps []step) {
		t.Helper()

		diagnostic, err := newUsageDiagnostic(dbPath, game, writer, time.UTC)
		if err != nil {
			t.Fatalf("open usage diagnostic: %v", err)
		}
		defer func() {
			if err := diagnostic.Close(); err != nil {
				t.Errorf("close usage diagnostic: %v", err)
			}
		}()

		for _, s := range steps {
			at := start.Add(time.Duration(s.second) * time.Second)
			got, err := diagnostic.Observe(tracking.Observation{
				At:             at,
				SessionID:      "12",
				SID:            sid,
				ExecutablePath: s.path,
				State:          s.state,
			})
			if err != nil {
				t.Fatalf("observe at +%ds: %v", s.second, err)
			}
			if got != s.want {
				t.Errorf("usage at +%ds = %s, want %s", s.second, got, s.want)
			}
		}
	}

	run([]step{
		{0, game, tracking.ObservationForeground, 0},
		{5, writer, tracking.ObservationForeground, 5 * time.Second},
		{10, "", tracking.ObservationInactive, 5 * time.Second},
		{15, game, tracking.ObservationForeground, 5 * time.Second},
		{20, "", tracking.ObservationInactive, 10 * time.Second},
	})

	run([]step{
		{23, game, tracking.ObservationForeground, 10 * time.Second},
		{28, writer, tracking.ObservationForeground, 15 * time.Second},
	})
}
