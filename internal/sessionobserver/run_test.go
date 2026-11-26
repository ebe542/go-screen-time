package sessionobserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestRunMonitorForwardsObservationsAndReadErrorsInOrder(t *testing.T) {
	const id = "12"
	const sid = "S-1-5-21-100-200-300-1001"
	const path = `C:\Games\Example\game.exe`

	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	readErr := errors.New("foreground read failed")
	session := NewSessionTracker(id, sid)
	session.Apply(SessionEvent{SessionID: id, Kind: SessionActivated})
	session.Apply(SessionEvent{SessionID: id, Kind: SessionUnlocked})

	reads := 0
	source := foregroundSourceFunc(func() (ForegroundProcess, error) {
		reads++
		if reads == 1 {
			return ForegroundProcess{}, readErr
		}
		return ForegroundProcess{SessionID: id, SID: sid, ExecutablePath: path}, nil
	})

	events := make(chan MonitorEvent, 4)
	events <- MonitorEvent{At: at, Kind: MonitorStart}
	events <- MonitorEvent{
		At: at.Add(time.Second), Kind: MonitorSessionChanged,
		Session: SessionEvent{SessionID: "13", Kind: SessionLocked},
	}
	events <- MonitorEvent{At: at.Add(5 * time.Second), Kind: MonitorHeartbeat}
	events <- MonitorEvent{
		At: at.Add(6 * time.Second), Kind: MonitorSessionChanged,
		Session: SessionEvent{SessionID: id, Kind: SessionLocked},
	}
	close(events)

	type delivery struct {
		observation tracking.Observation
		readErr     error
	}
	var received []delivery
	err := RunMonitor(
		context.Background(), NewMonitor(session, source), events,
		func(observation tracking.Observation, err error) error {
			received = append(received, delivery{observation, err})
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunMonitor: %v", err)
	}

	want := []delivery{
		{tracking.Observation{
			At: at, SessionID: id, SID: sid,
			State: tracking.ObservationUnavailable,
		}, readErr},
		{tracking.Observation{
			At: at.Add(5 * time.Second), SessionID: id, SID: sid,
			ExecutablePath: path, State: tracking.ObservationForeground,
		}, nil},
		{tracking.Observation{
			At: at.Add(6 * time.Second), SessionID: id, SID: sid,
			State: tracking.ObservationInactive,
		}, nil},
	}
	if len(received) != len(want) {
		t.Fatalf("received %d deliveries, want %d", len(received), len(want))
	}
	for i := range want {
		if received[i].observation != want[i].observation ||
			!errors.Is(received[i].readErr, want[i].readErr) {
			t.Errorf("delivery %d = %+v, want %+v", i, received[i], want[i])
		}
	}
}

func TestRunMonitorStopsAfterDeliveryError(t *testing.T) {
	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)
	events := make(chan MonitorEvent, 2)
	events <- MonitorEvent{At: at, Kind: MonitorStart}
	events <- MonitorEvent{At: at.Add(5 * time.Second), Kind: MonitorHeartbeat}
	close(events)

	monitor := NewMonitor(NewSessionTracker("12", "child-sid"), nil)
	deliveryErr := errors.New("observation receiver unavailable")
	calls := 0

	err := RunMonitor(
		context.Background(), monitor, events,
		func(tracking.Observation, error) error {
			calls++
			return deliveryErr
		},
	)
	if !errors.Is(err, deliveryErr) || calls != 1 {
		t.Errorf("RunMonitor error = %v, deliveries = %d, want %v and 1",
			err, calls, deliveryErr)
	}
}

func TestRunMonitorStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	monitor := NewMonitor(NewSessionTracker("12", "child-sid"), nil)
	err := RunMonitor(ctx, monitor, nil, func(tracking.Observation, error) error {
		t.Fatal("cancelled monitor delivered an observation")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RunMonitor error = %v, want context.Canceled", err)
	}
}
