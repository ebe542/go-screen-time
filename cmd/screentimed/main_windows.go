// Command screentimed runs the local observation service on Windows.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"

	"github.com/ebe542/go-screen-time/internal/observationservice"
)

const windowsServiceName = "GoScreenTime"

func main() {
	configPath := flag.String("config", "", "path to the child profile JSON file")
	dbPath := flag.String("db", "", "path to the SQLite usage database")
	pipeName := flag.String("pipe", `\\.\pipe\go-screen-time-observations`, "local observation pipe")
	flag.Parse()

	if *configPath == "" || *dbPath == "" {
		log.Fatal("-config and -db are required")
	}

	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("detect Windows service mode: %v", err)
	}
	if isService {
		handler := serviceHandler{
			run: func(ctx context.Context, ready func()) error {
				if !filepath.IsAbs(*configPath) || !filepath.IsAbs(*dbPath) {
					return errors.New("windows service requires absolute -config and -db paths")
				}
				return observationservice.RunWithReadiness(
					ctx, *configPath, *dbPath, *pipeName, time.Local, logReport, ready,
				)
			},
		}
		if err := svc.Run(windowsServiceName, handler); err != nil {
			log.Fatalf("run Windows service: %v", err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err = observationservice.RunConsole(ctx, *configPath, *dbPath, *pipeName, time.Local, logReport)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func logReport(report observationservice.Report) {
	if report.Err != nil {
		log.Printf("connection %d: %v", report.ConnectionID, report.Err)
	}
	if report.Disconnected {
		log.Printf("connection %d closed", report.ConnectionID)
		return
	}
	if report.Decision != "" {
		log.Printf("connection %d: %s %q", report.ConnectionID,
			report.Decision, report.Observation.ExecutablePath)
	}
}
