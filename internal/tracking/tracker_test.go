package tracking

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/policy"
)

const (
	childA = "S-1-5-21-100-200-300-1001"
	childB = "S-1-5-21-100-200-300-1002"

	gameA  = `C:\Games\A\game.exe`
	gameB  = `C:\Games\B\game.exe`
	writer = `C:\School\writer.exe`
)

var trackingStart = time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

func TestTrackerBooksOnlyConfirmedRestrictedForeground(t *testing.T) {
	tests := []struct {
		name         string
		observations []Observation
		want         []booking
	}{
		{
			name: "confirmation then app switch and unrestricted work",
			observations: []Observation{
				foreground(0, "session-a", childA, gameA),
				foreground(5, "session-a", childA, gameA),
				foreground(10, "session-a", childA, gameB),
				foreground(15, "session-a", childA, writer),
				foreground(20, "session-a", childA, writer),
				foreground(25, "session-a", childA, `C:\Other\game.exe`),
			},
			want: []booking{
				interval(childA, "game-a", 0, 5),
				interval(childA, "game-a", 5, 10),
				interval(childA, "game-b", 10, 15),
			},
		},
		{
			name: "account switch then lock and unlock",
			observations: []Observation{
				foreground(0, "session-a", childA, gameA),
				foreground(5, "session-b", childB, gameA),
				{At: afterSeconds(10), SessionID: "session-b", SID: childB, State: ObservationInactive},
				foreground(15, "session-b", childB, gameA),
				foreground(20, "session-b", childB, gameA),
			},
			want: []booking{
				interval(childA, "game-a", 0, 5),
				interval(childB, "game-a", 5, 10),
				interval(childB, "game-a", 15, 20),
			},
		},
		{
			name: "fifteen seconds is continuous and sixteen is a gap",
			observations: []Observation{
				foreground(0, "session-a", childA, gameA),
				foreground(15, "session-a", childA, gameA),
				foreground(31, "session-a", childA, gameA),
				foreground(36, "session-a", childA, gameA),
			},
			want: []booking{
				interval(childA, "game-a", 0, 15),
				interval(childA, "game-a", 31, 36),
			},
		},
		{
			name: "explicit observation loss discards the open interval",
			observations: []Observation{
				foreground(0, "session-a", childA, gameA),
				{At: afterSeconds(5), SessionID: "session-a", SID: childA, State: ObservationUnavailable},
				foreground(10, "session-a", childA, gameB),
				foreground(15, "session-a", childA, gameB),
			},
			want: []booking{
				interval(childA, "game-b", 10, 15),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingStore{}
			tracker := NewTracker(testRegistry(t), store, 15*time.Second)

			for _, observation := range tt.observations {
				if err := tracker.Observe(observation); err != nil {
					t.Fatalf("Observe at %v: %v", observation.At, err)
				}
			}

			if !reflect.DeepEqual(store.bookings, tt.want) {
				t.Errorf("bookings = %+v, want %+v", store.bookings, tt.want)
			}
		})
	}
}

func TestTrackerRetriesTheSameIntervalAfterWriteFailure(t *testing.T) {
	writeErr := errors.New("usage store write failed")
	store := &recordingStore{err: writeErr}
	tracker := NewTracker(testRegistry(t), store, 15*time.Second)

	if err := tracker.Observe(foreground(0, "session-a", childA, gameA)); err != nil {
		t.Fatalf("first observation: %v", err)
	}

	switchToB := foreground(5, "session-a", childA, gameB)
	if err := tracker.Observe(switchToB); !errors.Is(err, writeErr) {
		t.Fatalf("Observe error = %v, want %v", err, writeErr)
	}
	if len(store.bookings) != 0 {
		t.Fatalf("failed write produced bookings: %+v", store.bookings)
	}

	store.err = nil
	if err := tracker.Observe(switchToB); err != nil {
		t.Fatalf("retry observation: %v", err)
	}
	if err := tracker.Observe(foreground(10, "session-a", childA, writer)); err != nil {
		t.Fatalf("observe unrestricted app: %v", err)
	}

	want := []booking{
		interval(childA, "game-a", 0, 5),
		interval(childA, "game-b", 5, 10),
	}
	if !reflect.DeepEqual(store.bookings, want) {
		t.Errorf("bookings = %+v, want %+v", store.bookings, want)
	}
}

func testRegistry(t *testing.T) *managedapps.Registry {
	t.Helper()

	timed := &policy.AppAccessPolicy{}
	registry, err := managedapps.NewRegistry([]managedapps.ChildProfile{
		{
			SID: childA,
			Apps: []managedapps.App{
				{ID: "game-a", ExecutablePaths: []string{gameA}, Policy: timed},
				{ID: "game-b", ExecutablePaths: []string{gameB}, Policy: timed},
				{ID: "writer", ExecutablePaths: []string{writer}},
			},
		},
		{
			SID: childB,
			Apps: []managedapps.App{
				{ID: "game-a", ExecutablePaths: []string{gameA}, Policy: timed},
			},
		},
	})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}
	return registry
}

func foreground(seconds int, sessionID, sid, executable string) Observation {
	return Observation{
		At:             afterSeconds(seconds),
		SessionID:      sessionID,
		SID:            sid,
		ExecutablePath: executable,
		State:          ObservationForeground,
	}
}

func afterSeconds(seconds int) time.Time {
	return trackingStart.Add(time.Duration(seconds) * time.Second)
}

type booking struct {
	childID string
	appID   string
	from    time.Time
	to      time.Time
}

func interval(childID, appID string, from, to int) booking {
	return booking{childID: childID, appID: appID, from: afterSeconds(from), to: afterSeconds(to)}
}

type recordingStore struct {
	bookings []booking
	err      error
}

func (s *recordingStore) RecordAppUsageWithGrants(
	childID, appID string,
	from, to time.Time,
	_ policy.AppAccessPolicy,
) error {
	if s.err != nil {
		return s.err
	}
	s.bookings = append(s.bookings, booking{
		childID: childID,
		appID:   appID,
		from:    from,
		to:      to,
	})
	return nil
}
