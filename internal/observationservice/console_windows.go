package observationservice

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ebe542/go-screen-time/internal/control"
	"github.com/ebe542/go-screen-time/internal/observationpipe"
	"github.com/ebe542/go-screen-time/internal/storage/sqlite"
)

const maximumObservationGap = 15 * time.Second

// RunWithReadiness starts observation processing and reports when the pipe is open.
func RunWithReadiness(
	ctx context.Context,
	configPath, dbPath, pipeName string,
	location *time.Location,
	report func(Report),
	ready func(),
) error {
	config, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("open child profiles: %w", err)
	}
	registry, loadErr := LoadRegistry(config)
	closeErr := config.Close()
	if loadErr != nil {
		return fmt.Errorf("load child profiles: %w", loadErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close child profiles: %w", closeErr)
	}

	store, err := sqlite.Open(dbPath, location)
	if err != nil {
		return fmt.Errorf("open usage ledger: %w", err)
	}
	defer store.Close()

	controller, err := control.NewController(registry, store, maximumObservationGap)
	if err != nil {
		return fmt.Errorf("start access controller: %w", err)
	}
	listener, err := observationpipe.Listen(pipeName)
	if err != nil {
		return fmt.Errorf("listen for session helpers: %w", err)
	}
	defer listener.Close()

	if err := ctx.Err(); err != nil {
		return err
	}
	if ready != nil {
		ready()
	}
	return Serve(ctx, listener, control.NewObservationIntake(controller),
		activeConsoleSession, time.Now, report)
}

// RunConsole loads the child profiles and starts the local observation service.
func RunConsole(
	ctx context.Context,
	configPath, dbPath, pipeName string,
	location *time.Location,
	report func(Report),
) error {
	return RunWithReadiness(ctx, configPath, dbPath, pipeName, location, report, nil)
}

func activeConsoleSession() (string, error) {
	sessionID := windows.WTSGetActiveConsoleSessionId()
	if sessionID == 0xffffffff {
		return "", nil
	}
	return strconv.FormatUint(uint64(sessionID), 10), nil
}
