// Command screentimed runs the local observation service on Windows.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/ebe542/go-screen-time/internal/observationservice"
)

func main() {
	configPath := flag.String("config", "", "path to the child profile JSON file")
	dbPath := flag.String("db", "", "path to the SQLite usage database")
	pipeName := flag.String("pipe", `\\.\pipe\go-screen-time-observations`, "local observation pipe")
	flag.Parse()

	if *configPath == "" || *dbPath == "" {
		log.Fatal("-config and -db are required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := observationservice.RunConsole(ctx, *configPath, *dbPath, *pipeName, time.Local, logReport)
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
