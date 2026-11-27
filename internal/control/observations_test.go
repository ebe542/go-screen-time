package control

import (
	"testing"
	"time"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestObservationRouterRejectsMismatchedIdentity(t *testing.T) {
	processor := &recordingObservationProcessor{}
	router := NewObservationRouter(processor)
	connection := ObservationConnection{ID: 1, SessionID: "12", SID: childSID}

	if err := router.Activate(connection, start); err != nil {
		t.Fatalf("activate session: %v", err)
	}

	observation := foregroundObservation(0, gameA)
	wrongAccount := observation
	wrongAccount.SID = "S-1-5-21-100-200-300-1002"
	wrongSession := observation
	wrongSession.SessionID = "13"

	for _, submitted := range []tracking.Observation{wrongAccount, wrongSession} {
		got, err := router.Accept(connection.ID, submitted)
		if err == nil || got != "" {
			t.Errorf("mismatched observation = (%q, %v), want no decision and an error", got, err)
		}
	}

	if len(processor.observations) != 0 {
		t.Fatalf("mismatched observations reached the controller: %+v", processor.observations)
	}

	got, err := router.Accept(connection.ID, observation)
	if err != nil || got != managedapps.AccessAllowed {
		t.Errorf("valid observation = (%q, %v), want allowed", got, err)
	}
}

func TestObservationRouterEndsPreviousSessionAndIgnoresLateReports(t *testing.T) {
	processor := &recordingObservationProcessor{}
	router := NewObservationRouter(processor)
	first := ObservationConnection{ID: 1, SessionID: "12", SID: childSID}
	second := ObservationConnection{ID: 2, SessionID: "13", SID: childSID}

	if err := router.Activate(first, start); err != nil {
		t.Fatalf("activate first session: %v", err)
	}
	initial := foregroundObservation(0, gameA)
	if _, err := router.Accept(first.ID, initial); err != nil {
		t.Fatalf("accept first observation: %v", err)
	}

	switchedAt := start.Add(5 * time.Second)
	if err := router.Activate(second, switchedAt); err != nil {
		t.Fatalf("activate second session: %v", err)
	}

	// Even a newer timestamp cannot make the old connection active again.
	late := initial
	late.At = switchedAt.Add(time.Second)
	got, err := router.Accept(first.ID, late)
	if err != nil || got != "" {
		t.Errorf("late observation = (%q, %v), want no decision", got, err)
	}
	if err := router.Disconnect(first.ID, late.At); err != nil {
		t.Fatalf("ignore disconnect from old connection: %v", err)
	}

	current := late
	current.SessionID = second.SessionID
	current.ExecutablePath = gameB
	got, err = router.Accept(second.ID, current)
	if err != nil || got != managedapps.AccessAllowed {
		t.Errorf("second session observation = (%q, %v), want allowed", got, err)
	}

	assertRoutedObservations(t, processor.observations, []tracking.Observation{
		initial,
		{At: switchedAt, SessionID: first.SessionID, SID: first.SID, State: tracking.ObservationInactive},
		current,
	})
}

func TestObservationRouterReportsGapBeforeReconnect(t *testing.T) {
	processor := &recordingObservationProcessor{}
	router := NewObservationRouter(processor)
	first := ObservationConnection{ID: 1, SessionID: "12", SID: childSID}

	if err := router.Activate(first, start); err != nil {
		t.Fatalf("activate session: %v", err)
	}
	initial := foregroundObservation(0, gameA)
	if _, err := router.Accept(first.ID, initial); err != nil {
		t.Fatalf("accept initial observation: %v", err)
	}

	disconnectedAt := start.Add(5 * time.Second)
	if err := router.Disconnect(first.ID, disconnectedAt); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	reconnected := first
	reconnected.ID = 2
	resumed := initial
	resumed.At = start.Add(8 * time.Second)
	if err := router.Activate(reconnected, resumed.At); err != nil {
		t.Fatalf("reactivate session: %v", err)
	}
	if got, err := router.Accept(first.ID, resumed); err != nil || got != "" {
		t.Errorf("report from replaced connection = (%q, %v), want no decision", got, err)
	}
	if got, err := router.Accept(reconnected.ID, resumed); err != nil || got != managedapps.AccessAllowed {
		t.Errorf("report after reconnect = (%q, %v), want allowed", got, err)
	}

	assertRoutedObservations(t, processor.observations, []tracking.Observation{
		initial,
		{At: disconnectedAt, SessionID: first.SessionID, SID: first.SID, State: tracking.ObservationUnavailable},
		resumed,
	})
}

type recordingObservationProcessor struct {
	observations []tracking.Observation
}

func (p *recordingObservationProcessor) Process(
	observation tracking.Observation,
) (managedapps.AccessDecision, error) {
	p.observations = append(p.observations, observation)
	if observation.State == tracking.ObservationForeground {
		return managedapps.AccessAllowed, nil
	}
	return "", nil
}

func assertRoutedObservations(t *testing.T, got, want []tracking.Observation) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("routed observations = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("observation %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
