package observationservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ebe542/go-screen-time/internal/control"
	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/observationpipe"
	"github.com/ebe542/go-screen-time/internal/policy"
	"github.com/ebe542/go-screen-time/internal/storage/sqlite"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestServeBooksWithServiceTimeAndLeavesReconnectGap(t *testing.T) {
	sid, sessionID := testWindowsIdentity(t)
	const executable = `C:\Games\Example\game.exe`
	start := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

	registry := testServiceRegistry(t, sid, executable)
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "usage.db"), time.UTC)
	if err != nil {
		t.Fatalf("open SQLite ledger: %v", err)
	}
	defer store.Close()

	controller, err := control.NewController(registry, store, 15*time.Second)
	if err != nil {
		t.Fatalf("create controller: %v", err)
	}

	name := fmt.Sprintf(`\\.\pipe\go-screen-time-service-test-%d-%d`,
		os.Getpid(), time.Now().UnixNano())
	listener, err := observationpipe.Listen(name)
	if err != nil {
		t.Fatalf("listen on test pipe: %v", err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var clock atomic.Int64
	clock.Store(start.UnixNano())
	var activeSession atomic.Value
	activeSession.Store(sessionID)

	reports := make(chan Report, 16)
	done := make(chan error, 1)
	go func() {
		done <- Serve(
			ctx,
			listener,
			control.NewObservationIntake(controller),
			func() (string, error) { return activeSession.Load().(string), nil },
			func() time.Time { return time.Unix(0, clock.Load()).UTC() },
			func(report Report) { reports <- report },
		)
	}()

	dial := func() *observationpipe.Sender {
		t.Helper()
		sender, err := observationpipe.Dial(ctx, name)
		if err != nil {
			t.Fatalf("dial observation service: %v", err)
		}
		t.Cleanup(func() { _ = sender.Close() })
		return sender
	}

	nextReport := func() Report {
		t.Helper()
		select {
		case report := <-reports:
			if report.Err != nil {
				t.Fatalf("service report: %v", report.Err)
			}
			return report
		case <-ctx.Done():
			t.Fatalf("wait for service report: %v", ctx.Err())
			return Report{}
		}
	}

	send := func(sender *observationpipe.Sender, seconds int, want managedapps.AccessDecision) Report {
		t.Helper()
		at := start.Add(time.Duration(seconds) * time.Second)
		clock.Store(at.UnixNano())

		observation := tracking.Observation{
			At:             start.AddDate(0, 0, 2),
			SessionID:      sessionID,
			SID:            sid,
			ExecutablePath: executable,
			State:          tracking.ObservationForeground,
		}
		if err := sender.Send(ctx, observation); err != nil {
			t.Fatalf("send observation: %v", err)
		}
		report := nextReport()
		if report.Disconnected || report.Decision != want || !report.Observation.At.Equal(at) {
			t.Errorf("report = %+v, want decision %q at %v", report, want, at)
		}
		return report
	}

	assertUsage := func(want time.Duration) {
		t.Helper()
		got, err := store.BudgetTotals(sid, "game", start)
		if err != nil {
			t.Fatalf("read usage: %v", err)
		}
		if got.SharedRegular != want || got.AppRegular != want {
			t.Errorf("usage = %+v, want %s shared and app regular", got, want)
		}
	}

	first := dial()
	firstReport := send(first, 0, managedapps.AccessAllowed)
	send(first, 5, managedapps.AccessAllowed)
	assertUsage(5 * time.Second)

	clock.Store(start.Add(8 * time.Second).UnixNano())
	if err := first.Close(); err != nil {
		t.Fatalf("close first connection: %v", err)
	}
	disconnected := nextReport()
	if !disconnected.Disconnected || disconnected.ConnectionID != firstReport.ConnectionID {
		t.Errorf("disconnect report = %+v, want first connection", disconnected)
	}

	second := dial()
	secondReport := send(second, 10, managedapps.AccessAllowed)
	if secondReport.ConnectionID == 0 || secondReport.ConnectionID == firstReport.ConnectionID {
		t.Errorf("reconnect reused connection ID %d", secondReport.ConnectionID)
	}
	send(second, 15, managedapps.AccessAllowed)
	assertUsage(10 * time.Second)

	// The service observes Windows selecting a different session without a helper.
	activeSession.Store(sessionID + "0")
	send(second, 17, "")
	send(second, 20, "")
	assertUsage(12 * time.Second)

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Serve returned %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("service did not stop after cancellation")
	}
}

func testWindowsIdentity(t *testing.T) (sid, sessionID string) {
	t.Helper()

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("read test process SID: %v", err)
	}
	var id uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &id); err != nil {
		t.Fatalf("read test process session ID: %v", err)
	}
	return user.User.Sid.String(), strconv.FormatUint(uint64(id), 10)
}

func testServiceRegistry(t *testing.T, sid, executable string) *managedapps.Registry {
	t.Helper()

	endOfDay, err := policy.NewTimeOfDay(24, 0)
	if err != nil {
		t.Fatalf("create end of day: %v", err)
	}
	window, err := policy.NewUsageWindow(policy.TimeOfDay{}, endOfDay)
	if err != nil {
		t.Fatalf("create usage window: %v", err)
	}

	registry, err := managedapps.NewRegistry([]managedapps.ChildProfile{{
		SID: sid,
		Apps: []managedapps.App{{
			ID:              "game",
			ExecutablePaths: []string{executable},
			Policy: &policy.AppAccessPolicy{
				Schedule: policy.UsageSchedule{
					WeekdayWindows: []policy.UsageWindow{window},
				},
				SharedBudget: policy.DailyBudget{WeekdayLimit: time.Hour},
			},
		}},
	}})
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}
	return registry
}
