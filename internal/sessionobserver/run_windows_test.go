package sessionobserver

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestReadWindowsSessionInfoForCurrentSession(t *testing.T) {
	id, sid := interactiveWindowsTestIdentity(t)
	session := NewSessionTracker(strconv.FormatUint(uint64(id), 10), sid)

	data, err := readWindowsSessionInfo(id)
	if err != nil {
		t.Fatalf("read session info from Windows: %v", err)
	}
	if err := applyWindowsSessionInfo(session, data); err != nil {
		t.Fatalf("apply Windows session info: %v", err)
	}
}

func TestRunWindowsMonitorEmitsStartAndLaterObservation(t *testing.T) {
	id, sid := interactiveWindowsTestIdentity(t)
	wantID := strconv.FormatUint(uint64(id), 10)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	type result struct {
		first tracking.Observation
		later tracking.Observation
		err   error
	}
	done := make(chan result, 1)

	go func() {
		var got result
		got.err = RunWindowsMonitor(ctx, func(observation tracking.Observation, _ error) error {
			if got.first.At.IsZero() {
				got.first = observation
				return nil
			}
			if observation.At.Sub(got.first.At) >= 4*time.Second {
				got.later = observation
				cancel()
			}
			return nil
		})
		done <- got
	}()

	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("RunWindowsMonitor returned %v, want context.Canceled", got.err)
		}
		if got.first.At.IsZero() || got.later.At.IsZero() {
			t.Fatalf("missing start or later observation: first=%+v later=%+v", got.first, got.later)
		}
		for _, observation := range []tracking.Observation{got.first, got.later} {
			if observation.SessionID != wantID || observation.SID != sid {
				t.Errorf("observation identity = (%q, %q), want (%q, %q)",
					observation.SessionID, observation.SID, wantID, sid)
			}
		}
	case <-time.After(18 * time.Second):
		t.Fatal("Windows monitor did not finish after its context deadline")
	}
}

func interactiveWindowsTestIdentity(t *testing.T) (uint32, string) {
	t.Helper()

	var id uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &id); err != nil {
		t.Fatalf("get test process session ID: %v", err)
	}
	if id == 0 {
		t.Skip("requires a Windows user session")
	}

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("get test process user: %v", err)
	}
	return id, user.User.Sid.String()
}
