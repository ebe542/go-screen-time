package observationservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/observationpipe"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestRunConsoleUsesWindowsConsoleIdentity(t *testing.T) {
	sid, sessionID := testWindowsIdentity(t)
	active := windows.WTSGetActiveConsoleSessionId()
	if active == 0xffffffff || sessionID != strconv.FormatUint(uint64(active), 10) {
		t.Skip("requires the active Windows console session")
	}

	const executable = `C:\School\writer.exe`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "profiles.json")
	dbPath := filepath.Join(dir, "usage.db")
	config := fmt.Sprintf(`{
		"children": [{
			"sid": %q,
			"apps": [{"id": "writer", "executable_paths": [%q]}]
		}]
	}`, sid, executable)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatalf("write profiles: %v", err)
	}

	pipeName := fmt.Sprintf(`\\.\pipe\go-screen-time-console-test-%d-%d`,
		os.Getpid(), time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reports := make(chan Report, 4)
	done := make(chan error, 1)
	go func() {
		done <- RunConsole(ctx, configPath, dbPath, pipeName, time.UTC, func(report Report) {
			reports <- report
		})
	}()

	var sender *observationpipe.Sender
	for sender == nil {
		var err error
		sender, err = observationpipe.Dial(ctx, pipeName)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			t.Fatalf("dial console pipe: %v", err)
		}
		select {
		case err := <-done:
			t.Fatalf("console stopped before accepting a connection: %v", err)
		case <-ctx.Done():
			t.Fatalf("wait for console pipe: %v", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	defer sender.Close()

	if err := sender.Send(ctx, tracking.Observation{
		SessionID:      sessionID,
		SID:            sid,
		ExecutablePath: executable,
		State:          tracking.ObservationForeground,
	}); err != nil {
		t.Fatalf("send observation: %v", err)
	}

	select {
	case report := <-reports:
		if report.Err != nil || report.Disconnected ||
			report.Decision != managedapps.AccessAllowed || report.Observation.At.IsZero() {
			t.Errorf("report = %+v, want allowed with service timestamp", report)
		}
	case err := <-done:
		t.Fatalf("console stopped before reporting a decision: %v", err)
	case <-ctx.Done():
		t.Fatalf("wait for access decision: %v", ctx.Err())
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("RunConsole error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("console did not stop after cancellation")
	}
}

func TestRunConsoleRejectsInvalidConfigBeforeOpeningDatabase(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "profiles.json")
	dbPath := filepath.Join(dir, "usage.db")
	if err := os.WriteFile(configPath, []byte(`{"unknown": true}`), 0600); err != nil {
		t.Fatalf("write invalid profiles: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := RunConsole(ctx, configPath, dbPath,
		`\\.\pipe\go-screen-time-invalid-config-test`, time.UTC, nil)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunConsole error = %v, want a configuration error", err)
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("database should not be created for invalid profiles, stat error = %v", err)
	}
}
